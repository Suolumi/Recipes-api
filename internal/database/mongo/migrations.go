package mongo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	translationMigrationID = "recipe_translations_v1"
	translationBackfillID  = "recipe_translations_populate_v1"
)

type schemaMigration struct {
	ID          string    `bson:"_id"`
	CompletedAt time.Time `bson:"completed_at"`
}

// TranslationBackfillCompleted reports whether the one-shot translate-existing-
// recipes pass has already finished.
func (c *Client) TranslationBackfillCompleted(ctx context.Context) (bool, error) {
	err := c.db.Collection("schema_migrations").FindOne(ctx, bson.M{"_id": translationBackfillID}).Err()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return false, err
}

// MarkTranslationBackfillCompleted records that the backfill pass finished
// cleanly so it never runs again.
func (c *Client) MarkTranslationBackfillCompleted(ctx context.Context) error {
	_, err := c.db.Collection("schema_migrations").UpdateOne(ctx, bson.M{"_id": translationBackfillID}, bson.M{
		"$setOnInsert": schemaMigration{ID: translationBackfillID, CompletedAt: time.Now()},
	}, options.Update().SetUpsert(true))
	return err
}

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

func (c *Client) MigrateLegacyTranslations(ctx context.Context) error {
	markers := c.db.Collection("schema_migrations")
	var marker schemaMigration
	if err := markers.FindOne(ctx, bson.M{"_id": translationMigrationID}).Decode(&marker); err == nil {
		return nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("read translation migration marker: %w", err)
	}

	names, err := c.db.ListCollectionNames(ctx, bson.M{"name": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(recipesCollection) + "_[A-Za-z0-9-]+$"}})
	if err != nil {
		return fmt.Errorf("list legacy translation collections: %w", err)
	}
	for _, name := range names {
		locale := strings.TrimPrefix(name, recipesCollection+"_")
		cursor, err := c.db.Collection(name).Find(ctx, bson.D{})
		if err != nil {
			return fmt.Errorf("read legacy translation collection %s: %w", name, err)
		}
		var documents []struct {
			ID              *primitive.ObjectID `bson:"_id,omitempty"`
			Author          *primitive.ObjectID `bson:"author,omitempty"`
			Title           string              `bson:"title,omitempty"`
			Description     string              `bson:"description,omitempty"`
			Quantity        int                 `bson:"quantity,omitempty"`
			Kind            string              `bson:"kind,omitempty"`
			PreparationTime int                 `bson:"preparation_time,omitempty"`
			CookingTime     int                 `bson:"cooking_time,omitempty"`
			RestingTime     int                 `bson:"resting_time,omitempty"`
			Ingredients     []bson.M            `bson:"ingredients,omitempty"`
			Steps           []bson.M            `bson:"steps,omitempty"`
			Pictures        []string            `bson:"pictures,omitempty"`
			SourceLocale    string              `bson:"source_locale,omitempty"`
			SourceHash      string              `bson:"source_hash,omitempty"`
		}
		if err := cursor.All(ctx, &documents); err != nil {
			return fmt.Errorf("decode legacy translation collection %s: %w", name, err)
		}
		for _, document := range documents {
			if document.ID == nil {
				return fmt.Errorf("legacy translation collection %s contains a document without an id", name)
			}
			translation := bson.M{
				"_id": primitive.NewObjectID(), "recipe_id": *document.ID, "author": document.Author,
				"title": document.Title, "description": document.Description, "quantity": document.Quantity,
				"kind": document.Kind, "preparation_time": document.PreparationTime, "cooking_time": document.CookingTime,
				"resting_time": document.RestingTime, "ingredients": document.Ingredients, "steps": document.Steps,
				"pictures": document.Pictures, "source_locale": document.SourceLocale, "locale": locale, "source_hash": document.SourceHash,
			}
			filter := bson.M{"recipe_id": *document.ID, "locale": locale}
			var existing struct {
				ID primitive.ObjectID `bson:"_id"`
			}
			findErr := c.db.Collection(translationsCollection).FindOne(ctx, filter).Decode(&existing)
			if findErr == nil {
				translation["_id"] = existing.ID
			} else if !errors.Is(findErr, mongo.ErrNoDocuments) {
				return fmt.Errorf("find existing translation for %s recipe %s: %w", name, document.ID.Hex(), findErr)
			}
			if _, err := c.db.Collection(translationsCollection).ReplaceOne(ctx, filter, translation, options.Replace().SetUpsert(true)); err != nil {
				return fmt.Errorf("migrate %s recipe %s: %w", name, document.ID.Hex(), err)
			}
		}
	}
	if _, err := markers.UpdateOne(ctx, bson.M{"_id": translationMigrationID}, bson.M{
		"$setOnInsert": schemaMigration{ID: translationMigrationID, CompletedAt: time.Now()},
	}, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("write translation migration marker: %w", err)
	}
	return nil
}
