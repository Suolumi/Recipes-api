package mongo

import (
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/exp/slices"
	"golang.org/x/net/context"
	"recipes/internal/models"
	"recipes/internal/utils"
	"reflect"
	"strings"
)

const recipesCollection string = "recipes"

var NotFoundError = errors.New("recipe not found")

var recipeAuthorPipeline = mongo.Pipeline{
	bson.D{{"$lookup", bson.D{
		{"from", userCollection},
		{"localField", "author"},
		{"foreignField", "_id"},
		{"as", "author"},
	}}},
	bson.D{{"$unwind", "$author"}},
}

func getLocaleCollection(locale string) string {
	return fmt.Sprintf("%s_%s", recipesCollection, locale)
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

func (c *Client) AddLocaleRecipe(recipe models.Recipe, locale string) (models.Recipe, error) {
	if locale == "" {
		locale = "en"
	}

	cursor, err := c.db.Collection(getLocaleCollection(locale)).InsertOne(context.TODO(), recipe.ToRecipeDB())
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
	if err := cursor.Decode(&recipe); errors.Is(err, mongo.ErrNoDocuments) {
		return models.Recipe{}, fmt.Errorf("recipe not found")
	}
	return c.GetRecipeById(id)
}

func (c *Client) getRecipeById(id string, collection string) (models.Recipe, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Recipe{}, err
	}

	var stages mongo.Pipeline = append([]bson.D{{{"$match", bson.D{
		{"_id", objectId},
	}}}}, recipeAuthorPipeline...)

	cursor, err := c.db.Collection(collection).Aggregate(context.TODO(), stages)
	if err != nil {
		return models.Recipe{}, err
	}
	recipes, err := c.transformRecipe(cursor)
	if err != nil {
		return models.Recipe{}, err
	}
	if recipes == nil {
		return models.Recipe{}, NotFoundError
	}
	return recipes[0], nil
}

func (c *Client) GetRecipeById(id string) (models.Recipe, error) {
	return c.getRecipeById(id, recipesCollection)
}

func (c *Client) GetRecipeByIdLocale(id string, locale string) (models.Recipe, error) {
	return c.getRecipeById(id, getLocaleCollection(locale))
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
	}
	return recipe, nil
}

func (c *Client) GetRecipes(parameters models.GetRecipesRequest) ([]models.RecipePreview, int64, error) {
	var recipes []models.RecipePreview
	var pipeline []bson.D

	pipeline = append(pipeline, recipeAuthorPipeline...)

	if parameters.Title != "" {
		pipeline = append(pipeline, bson.D{{"$match", bson.D{{"title", primitive.Regex{Pattern: parameters.Title, Options: "i"}}}}})
	}
	if parameters.Author != "" {
		pipeline = append(pipeline, bson.D{{"$match", bson.D{{"author.username", primitive.Regex{Pattern: parameters.Author, Options: "i"}}}}})
	}
	if parameters.Kind != "" {
		pipeline = append(pipeline, bson.D{{"$match", bson.D{{"kind", primitive.Regex{Pattern: string(parameters.Kind), Options: "i"}}}}})
	}

	if len(parameters.Ingredients) > 0 {
		pipeline = append(pipeline, bson.D{{"$match", bson.M{"ingredients.name": bson.M{"$all": parameters.Ingredients}}}})
	}

	// Count total number of documents before sorting without limit and skip
	cursor, err := c.db.Collection(recipesCollection).Aggregate(context.TODO(), append(slices.Clone(pipeline), bson.D{{"$count", "total"}}))
	if err != nil {
		return nil, 0, err
	}

	var count int64 = 0
	for cursor.Next(context.Background()) {
		count++
	}

	if parameters.PreparationTime != 0 || parameters.TotalTime != 0 {
		if parameters.PreparationTime != 0 {
			pipeline = append(pipeline, bson.D{{"$addFields", bson.D{
				{"diffPrepTime", bson.D{
					{"$abs", bson.A{
						bson.D{{"$subtract", bson.A{"$preparationTime", parameters.PreparationTime}}},
					}},
				}},
			}}})
		}
		if parameters.TotalTime != 0 {
			pipeline = append(pipeline, bson.D{{"$addFields", bson.D{
				{"diffTotalTime", bson.D{
					{"$abs", bson.A{
						bson.D{{"$subtract", bson.A{
							bson.D{{"$add", bson.A{"$preparationTime", "$cookingTime", "$restingTime"}}}, // Sum of the three fields
							parameters.TotalTime,
						}}},
					}},
				}},
			}}})
		}
		pipeline = append(pipeline, bson.D{{"$addFields", bson.D{
			{"combinedDifference", bson.D{
				{"$add", bson.A{"$diffPrepTime", "$diffTotalTime"}},
			}},
		}}}, bson.D{{"$sort", bson.D{
			{"combinedDifference", 1},
		}}})
	}

	// Apply limit and offset for pagination
	if parameters.Limit != 0 {
		pipeline = append(pipeline, bson.D{{"$limit", parameters.Limit}})
	} else {
		pipeline = append(pipeline, bson.D{{"$limit", 15}})
	}
	if parameters.Offset != 0 {
		pipeline = append(pipeline, bson.D{{"$skip", parameters.Offset}})
	}

	cursor, err = c.db.Collection(recipesCollection).Aggregate(context.TODO(), pipeline)
	if err != nil {
		return nil, 0, err
	}

	for cursor.Next(context.Background()) {
		var result models.RecipePreview
		if err := cursor.Decode(&result); err != nil {
			return nil, 0, err
		}
		recipes = append(recipes, result)
	}

	return recipes, count, nil
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

		if cursor := c.db.Collection(recipesCollection).FindOne(context.TODO(), bson.M{bsonName: field.Interface()}); !errors.Is(cursor.Decode(&dbRecipe), mongo.ErrNoDocuments) {
			return dbRecipe, fmt.Errorf("%s is already taken", jsonName)
		}
	}

	return models.RecipeDB{}, nil
}
