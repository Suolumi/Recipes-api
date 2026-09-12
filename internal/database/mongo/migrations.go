package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (c *Client) EnsureIndexes(ctx context.Context) error {
	users := c.db.Collection(userCollection)
	if _, err := users.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true).SetName("users_username_unique")},
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).SetName("users_email_unique")},
	}); err != nil {
		return fmt.Errorf("create user indexes: %w", err)
	}
	if _, err := c.db.Collection(recipesCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "author", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("recipes_author_cursor"),
	}); err != nil {
		return fmt.Errorf("create recipe indexes: %w", err)
	}
	if _, err := c.db.Collection(translationsCollection).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "recipe_id", Value: 1}, {Key: "locale", Value: 1}}, Options: options.Index().SetUnique(true).SetName("recipe_translations_recipe_locale_unique")},
		{Keys: bson.D{{Key: "locale", Value: 1}, {Key: "title", Value: 1}}, Options: options.Index().SetName("recipe_translations_locale_title")},
	}); err != nil {
		return fmt.Errorf("create translation indexes: %w", err)
	}
	return nil
}
