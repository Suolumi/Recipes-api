package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"recipes/internal/models"
)

const favoritesCollection = "favorites"

// AddFavorite is idempotent: favoriting an already-favorited recipe is a no-op.
func (c *Client) AddFavorite(ctx context.Context, userID, recipeID string) error {
	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return err
	}
	recipeObjectID, err := primitive.ObjectIDFromHex(recipeID)
	if err != nil {
		return err
	}
	_, err = c.db.Collection(favoritesCollection).UpdateOne(ctx,
		bson.M{"recipe": recipeObjectID},
		bson.M{"$addToSet": bson.M{"users": userObjectID}},
		options.Update().SetUpsert(true),
	)
	return err
}

// RemoveFavorite is idempotent: it doesn't error when the recipe was never
// favorited, or was already un-favorited.
func (c *Client) RemoveFavorite(ctx context.Context, userID, recipeID string) error {
	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return err
	}
	recipeObjectID, err := primitive.ObjectIDFromHex(recipeID)
	if err != nil {
		return err
	}
	_, err = c.db.Collection(favoritesCollection).UpdateOne(ctx,
		bson.M{"recipe": recipeObjectID},
		bson.M{"$pull": bson.M{"users": userObjectID}},
	)
	return err
}

func (c *Client) DeleteFavoritesByRecipeID(ctx context.Context, recipeID string) error {
	recipeObjectID, err := primitive.ObjectIDFromHex(recipeID)
	if err != nil {
		return err
	}
	_, err = c.db.Collection(favoritesCollection).DeleteOne(ctx, bson.M{"recipe": recipeObjectID})
	return err
}

// GetFavoriteInfo returns, for each given recipe id, its public favorite count
// and (when userID is non-empty) whether that user has favorited it. A recipe
// never favorited is simply absent from the map; callers should treat a
// missing entry the same as {Count: 0, Favorited: false}.
func (c *Client) GetFavoriteInfo(ctx context.Context, ids []string, userID string) (map[string]models.FavoriteInfo, error) {
	objectIDs := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		objectID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, objectID)
	}
	result := make(map[string]models.FavoriteInfo, len(objectIDs))
	if len(objectIDs) == 0 {
		return result, nil
	}

	var userObjectID primitive.ObjectID
	if userID != "" {
		var err error
		userObjectID, err = primitive.ObjectIDFromHex(userID)
		if err != nil {
			return nil, err
		}
	}

	cursor, err := c.db.Collection(favoritesCollection).Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "recipe", Value: bson.D{{Key: "$in", Value: objectIDs}}}}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "recipe", Value: 1},
			{Key: "count", Value: bson.D{{Key: "$size", Value: "$users"}}},
			{Key: "favorited", Value: bson.D{{Key: "$in", Value: bson.A{userObjectID, "$users"}}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	var docs []struct {
		Recipe    primitive.ObjectID `bson:"recipe"`
		Count     int64              `bson:"count"`
		Favorited bool               `bson:"favorited"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		result[doc.Recipe.Hex()] = models.FavoriteInfo{Count: doc.Count, Favorited: userID != "" && doc.Favorited}
	}
	return result, nil
}

// favoritedRecipeIDs returns the family root id for every recipe userID has
// favorited, deduplicated - favoriting a variation still counts as favoriting
// its family for boosting purposes (decision #6), so this resolves each
// favorited recipe to its root (or itself, if it's already a root) rather
// than returning the raw favorited ids.
func (c *Client) favoritedRecipeIDs(ctx context.Context, userID string) ([]primitive.ObjectID, error) {
	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, err
	}
	cursor, err := c.db.Collection(favoritesCollection).Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "users", Value: userObjectID}}}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: recipesCollection},
			{Key: "localField", Value: "recipe"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "recipeDoc"},
		}}},
		bson.D{{Key: "$unwind", Value: "$recipeDoc"}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "root", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$recipeDoc.variation_of", "$recipeDoc._id"}}}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$root"}}}},
	})
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	return ids, nil
}

// GetFamilyFavoriteInfo is the family-aware counterpart to GetFavoriteInfo:
// for each given root id, it returns the number of distinct people who
// favorited that root or any of its variations (not a raw sum - the same
// person favoriting two versions of the same dish counts once, decision #6),
// and whether userID is one of them.
func (c *Client) GetFamilyFavoriteInfo(ctx context.Context, rootIDs []string, userID string) (map[string]models.FavoriteInfo, error) {
	result := make(map[string]models.FavoriteInfo, len(rootIDs))
	objectIDs := make([]primitive.ObjectID, 0, len(rootIDs))
	for _, id := range rootIDs {
		objectID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, objectID)
	}
	if len(objectIDs) == 0 {
		return result, nil
	}
	var userObjectID primitive.ObjectID
	if userID != "" {
		var err error
		userObjectID, err = primitive.ObjectIDFromHex(userID)
		if err != nil {
			return nil, err
		}
	}

	cursor, err := c.db.Collection(recipesCollection).Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: objectIDs}}}}}},
		variationsLookupStage,
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "family_ids", Value: bson.D{{Key: "$concatArrays", Value: bson.A{
				bson.A{"$_id"}, "$variations._id",
			}}}},
		}}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: favoritesCollection},
			{Key: "localField", Value: "family_ids"},
			{Key: "foreignField", Value: "recipe"},
			{Key: "as", Value: "family_favorites"},
		}}},
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "family_users", Value: bson.D{{Key: "$reduce", Value: bson.D{
				{Key: "input", Value: "$family_favorites.users"},
				{Key: "initialValue", Value: bson.A{}},
				{Key: "in", Value: bson.D{{Key: "$setUnion", Value: bson.A{"$$value", "$$this"}}}},
			}}}},
		}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "count", Value: bson.D{{Key: "$size", Value: "$family_users"}}},
			{Key: "favorited", Value: bson.D{{Key: "$in", Value: bson.A{userObjectID, "$family_users"}}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID        primitive.ObjectID `bson:"_id"`
		Count     int64              `bson:"count"`
		Favorited bool               `bson:"favorited"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		result[doc.ID.Hex()] = models.FavoriteInfo{Count: doc.Count, Favorited: userID != "" && doc.Favorited}
	}
	return result, nil
}
