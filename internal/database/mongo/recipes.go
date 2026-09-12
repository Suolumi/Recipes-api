package mongo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"

	"recipes/internal/models"
	"recipes/internal/utils"
)

const recipesCollection string = "recipes"

var NotFoundError = errors.New("recipe not found")

var recipeAuthorPipeline = mongo.Pipeline{
	bson.D{{Key: "$lookup", Value: bson.D{
		{Key: "from", Value: userCollection},
		{Key: "localField", Value: "author"},
		{Key: "foreignField", Value: "_id"},
		{Key: "as", Value: "author"},
	}}},
	bson.D{{Key: "$unwind", Value: "$author"}},
}

func (c *Client) transformRecipe(cursor *mongo.Cursor) ([]models.Recipe, error) {
	var results []bson.M
	var recipes []models.Recipe

	if err := cursor.All(context.TODO(), &results); err != nil {
		return nil, err
	}

	for _, result := range results {
		var recipe models.Recipe
		bsonBytes, err := bson.Marshal(result)

		if err != nil {
			return nil, err
		}
		if err := bson.Unmarshal(bsonBytes, &recipe); err != nil {
			return nil, err
		}
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func (c *Client) CreateRecipe(authorId string, infos *models.CreateRecipe) (models.Recipe, error) {
	authorObjectId, err := primitive.ObjectIDFromHex(authorId)
	if err != nil {
		return models.Recipe{}, err
	}

	recipe := utils.DupStruct[models.RecipeDB](infos)
	recipe.Author = &authorObjectId

	cursor, err := c.db.Collection(recipesCollection).InsertOne(context.TODO(), recipe)
	if err != nil {
		return models.Recipe{}, err
	}
	return c.GetRecipeById(cursor.InsertedID.(primitive.ObjectID).Hex())
}

func (c *Client) UpdateRecipeById(id string, recipe *models.UpdateRecipeRequest) (models.Recipe, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Recipe{}, err
	}

	cursor := c.db.Collection(recipesCollection).FindOneAndUpdate(context.TODO(), bson.M{
		"_id": objectId,
	}, bson.M{
		"$set": recipe,
	})
	var updated models.RecipeDB
	if err := cursor.Decode(&updated); errors.Is(err, mongo.ErrNoDocuments) {
		return models.Recipe{}, NotFoundError
	} else if err != nil {
		return models.Recipe{}, err
	}
	return c.GetRecipeById(id)
}

func (c *Client) getRecipeById(id string, collection string) (models.Recipe, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Recipe{}, err
	}

	var stages mongo.Pipeline = append([]bson.D{{
		{Key: "$match", Value: bson.D{{Key: "_id", Value: objectId}}},
	}}, recipeAuthorPipeline...)

	cursor, err := c.db.Collection(collection).Aggregate(context.TODO(), stages)
	if err != nil {
		return models.Recipe{}, err
	}
	recipes, err := c.transformRecipe(cursor)
	if err != nil {
		return models.Recipe{}, err
	}
	if len(recipes) == 0 {
		return models.Recipe{}, NotFoundError
	}
	return recipes[0], nil
}

func (c *Client) GetRecipeById(id string) (models.Recipe, error) {
	return c.getRecipeById(id, recipesCollection)
}

func (c *Client) GetRecipeByIdForAuthor(ctx context.Context, id, authorID string) (models.Recipe, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Recipe{}, err
	}
	authorObjectID, err := primitive.ObjectIDFromHex(authorID)
	if err != nil {
		return models.Recipe{}, err
	}
	stages := append(mongo.Pipeline{{
		{Key: "$match", Value: bson.D{{Key: "_id", Value: objectID}, {Key: "author", Value: authorObjectID}}},
	}}, recipeAuthorPipeline...)
	cursor, err := c.db.Collection(recipesCollection).Aggregate(ctx, stages)
	if err != nil {
		return models.Recipe{}, err
	}
	recipes, err := c.transformRecipe(cursor)
	if err != nil {
		return models.Recipe{}, err
	}
	if len(recipes) == 0 {
		return models.Recipe{}, NotFoundError
	}
	return recipes[0], nil
}

func (c *Client) ReplaceRecipeById(ctx context.Context, id string, recipe models.RecipeDB) (models.Recipe, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Recipe{}, err
	}
	recipe.Id = &objectID
	result := c.db.Collection(recipesCollection).FindOneAndReplace(ctx, bson.M{"_id": objectID}, recipe, options.FindOneAndReplace().SetReturnDocument(options.After))
	if err := result.Err(); errors.Is(err, mongo.ErrNoDocuments) {
		return models.Recipe{}, NotFoundError
	} else if err != nil {
		return models.Recipe{}, err
	}
	return c.GetRecipeById(id)
}

func (c *Client) DeleteLocalizedRecipesByID(ctx context.Context, id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = c.db.Collection(translationsCollection).DeleteMany(ctx, bson.M{"recipe_id": objectID})
	return err
}

func (c *Client) DeleteRecipeById(id string) (models.RecipeDB, error) {
	var recipe models.RecipeDB
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return recipe, err
	}

	cursor := c.db.Collection(recipesCollection).FindOneAndDelete(context.TODO(), bson.M{
		"_id": objectId,
	})
	if err := cursor.Decode(&recipe); errors.Is(err, mongo.ErrNoDocuments) {
		return recipe, NotFoundError
	} else if err != nil {
		return recipe, err
	}
	return recipe, nil
}

func (c *Client) GetRecipeDocuments(parameters models.GetRecipesRequest) ([]models.Recipe, int64, error) {
	var pipeline []bson.D

	pipeline = append(pipeline, recipeAuthorPipeline...)

	if parameters.SearchLocale != "" && (parameters.Title != "" || len(parameters.Ingredients) > 0) {
		translationMatch := bson.D{
			{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$recipe_id", "$$recipeID"}}}},
			{Key: "locale", Value: parameters.SearchLocale},
		}
		if parameters.Title != "" {
			translationMatch = append(translationMatch, bson.E{Key: "title", Value: primitive.Regex{Pattern: regexp.QuoteMeta(parameters.Title), Options: "i"}})
		}
		if len(parameters.Ingredients) > 0 {
			ingredients := make([]interface{}, 0, len(parameters.Ingredients))
			for _, ingredient := range parameters.Ingredients {
				ingredients = append(ingredients, primitive.Regex{Pattern: regexp.QuoteMeta(ingredient), Options: "i"})
			}
			translationMatch = append(translationMatch, bson.E{Key: "ingredients.name", Value: bson.M{"$all": ingredients}})
		}
		pipeline = append(pipeline, bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: translationsCollection},
			{Key: "let", Value: bson.D{{Key: "recipeID", Value: "$_id"}}},
			{Key: "pipeline", Value: mongo.Pipeline{{{Key: "$match", Value: translationMatch}}}},
			{Key: "as", Value: "translation_match"},
		}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "translation_match.0", Value: bson.D{{Key: "$exists", Value: true}}}}}}, bson.D{{Key: "$unset", Value: "translation_match"}})
	} else if parameters.Title != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "title", Value: primitive.Regex{Pattern: regexp.QuoteMeta(parameters.Title), Options: "i"}}}}})
	}
	if parameters.Author != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "author.username", Value: primitive.Regex{Pattern: regexp.QuoteMeta(parameters.Author), Options: "i"}}}}})
	}
	if parameters.Kind != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "kind", Value: primitive.Regex{Pattern: regexp.QuoteMeta(string(parameters.Kind)), Options: "i"}}}}})
	}

	if len(parameters.Ingredients) > 0 && parameters.SearchLocale == "" {
		ingredients := make([]interface{}, 0, len(parameters.Ingredients))
		for _, ingredient := range parameters.Ingredients {
			ingredients = append(ingredients, primitive.Regex{Pattern: regexp.QuoteMeta(ingredient), Options: "i"})
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "ingredients.name", Value: bson.M{"$all": ingredients}}}}})
	}

	// Count total number of documents before sorting without limit and skip
	cursor, err := c.db.Collection(recipesCollection).Aggregate(context.TODO(), append(slices.Clone(pipeline), bson.D{{Key: "$count", Value: "total"}}))
	if err != nil {
		return nil, 0, err
	}

	var count int64
	if cursor.Next(context.Background()) {
		var result struct {
			Total int64 `bson:"total"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, 0, err
		}
		count = result.Total
	}

	if parameters.PreparationTime != 0 || parameters.TotalTime != 0 {
		differenceFields := bson.A{}
		if parameters.PreparationTime != 0 {
			pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "diffPrepTime", Value: bson.D{
					{Key: "$abs", Value: bson.A{
						bson.D{{Key: "$subtract", Value: bson.A{"$preparation_time", parameters.PreparationTime}}},
					}},
				}},
			}}})
			differenceFields = append(differenceFields, "$diffPrepTime")
		}
		if parameters.TotalTime != 0 {
			pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "diffTotalTime", Value: bson.D{
					{Key: "$abs", Value: bson.A{
						bson.D{{Key: "$subtract", Value: bson.A{
							bson.D{{Key: "$add", Value: bson.A{"$preparation_time", "$cooking_time", "$resting_time"}}},
							parameters.TotalTime,
						}}},
					}},
				}},
			}}})
			differenceFields = append(differenceFields, "$diffTotalTime")
		}
		pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "combinedDifference", Value: bson.D{
				{Key: "$add", Value: differenceFields},
			}},
		}}}, bson.D{{Key: "$sort", Value: bson.D{
			{Key: "combinedDifference", Value: 1},
			{Key: "_id", Value: -1},
		}}})
	} else {
		pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}})
	}

	// Apply limit and offset for pagination
	if parameters.Offset != 0 {
		pipeline = append(pipeline, bson.D{{Key: "$skip", Value: parameters.Offset}})
	}
	if parameters.Limit != 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: parameters.Limit}})
	} else {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: 15}})
	}

	cursor, err = c.db.Collection(recipesCollection).Aggregate(context.TODO(), pipeline)
	if err != nil {
		return nil, 0, err
	}

	recipes, err := c.transformRecipe(cursor)
	return recipes, count, err
}

func (c *Client) GetRecipesByAuthor(ctx context.Context, authorID, cursorID string, limit int) ([]models.Recipe, int64, error) {
	authorObjectID, err := primitive.ObjectIDFromHex(authorID)
	if err != nil {
		return nil, 0, err
	}
	match := bson.D{{Key: "author", Value: authorObjectID}}
	if cursorID != "" {
		cursorObjectID, err := primitive.ObjectIDFromHex(cursorID)
		if err != nil {
			return nil, 0, err
		}
		match = append(match, bson.E{Key: "_id", Value: bson.M{"$lt": cursorObjectID}})
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}},
		{{Key: "$limit", Value: limit}},
	}
	pipeline = append(pipeline, recipeAuthorPipeline...)
	dbCursor, err := c.db.Collection(recipesCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	recipes, err := c.transformRecipe(dbCursor)
	if err != nil {
		return nil, 0, err
	}
	count, err := c.db.Collection(recipesCollection).CountDocuments(ctx, bson.M{"author": authorObjectID})
	return recipes, count, err
}

func (c *Client) RecipeConflicts(recipe models.RecipeDB) (models.RecipeDB, error) {
	val := reflect.ValueOf(&recipe).Elem()
	typ := val.Type()
	dbRecipe := models.RecipeDB{}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldInfos := typ.Field(i)

		if field.IsZero() {
			continue
		}

		jsonName := fieldInfos.Tag.Get("json")
		// Split to remove potential ,omitempty
		bsonName := strings.Split(fieldInfos.Tag.Get("bson"), ",")[0]

		if err := c.db.Collection(recipesCollection).FindOne(context.TODO(), bson.M{bsonName: field.Interface()}).Decode(&dbRecipe); err == nil {
			return dbRecipe, fmt.Errorf("%s is already taken", jsonName)
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return models.RecipeDB{}, fmt.Errorf("check recipe conflict: %w", err)
		}
	}

	return models.RecipeDB{}, nil
}
