package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"recipes/internal/config"
	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/models"
)

func TestGetRecipeDocumentsSearchesLocalizedTitle(t *testing.T) {
	databaseName := "Recipes_test_" + primitive.NewObjectID().Hex()
	db, err := mongorepo.New(&config.DatabaseConfig{
		DefaultAddr: "mongodb://localhost:27017",
		Name:        databaseName,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Skipf("local MongoDB is not available: %v", err)
	}
	defer func() {
		_ = db.RawDatabase().Drop(context.Background())
		_ = db.Close(context.Background())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, db.EnsureIndexes(ctx))

	authorID := primitive.NewObjectID()
	users := db.RawDatabase().Collection("users")
	_, err = users.InsertOne(ctx, bson.M{
		"_id": authorID, "username": "search-author", "email": "search@example.test",
	})
	require.NoError(t, err)

	recipeID := primitive.NewObjectID()
	recipes := db.RawDatabase().Collection("recipes")
	_, err = recipes.InsertOne(ctx, bson.M{
		"_id":         recipeID,
		"author":      authorID,
		"title":       "Pancakes",
		"ingredients": bson.A{bson.M{"name": "flour"}},
		"kind":        "dish",
	})
	require.NoError(t, err)

	translations := db.RawDatabase().Collection("recipe_translations")
	_, err = translations.InsertOne(ctx, bson.M{
		"_id": primitive.NewObjectID(), "recipe_id": recipeID, "locale": "fr", "title": "Crêpes",
	})
	require.NoError(t, err)

	localizedMatches, _, err := db.GetRecipeDocuments(models.GetRecipesRequest{Title: "crêpes", SearchLocale: "fr", Limit: 10})
	require.NoError(t, err)
	require.Len(t, localizedMatches, 1)
	assert.Equal(t, recipeID.Hex(), localizedMatches[0].Id.Hex())

	canonicalMatches, _, err := db.GetRecipeDocuments(models.GetRecipesRequest{Title: "pancakes", Limit: 10})
	require.NoError(t, err)
	require.Len(t, canonicalMatches, 1)
	assert.Equal(t, recipeID.Hex(), canonicalMatches[0].Id.Hex())
}

// TestAddLocaleRecipeReplacesExisting guards against the regression where
// AddLocaleRecipe stamped a fresh _id into the translation document, making the
// upsert fail on every replace because _id is immutable.
func TestAddLocaleRecipeReplacesExisting(t *testing.T) {
	databaseName := "Recipes_test_" + primitive.NewObjectID().Hex()
	db, err := mongorepo.New(&config.DatabaseConfig{
		DefaultAddr: "mongodb://localhost:27017",
		Name:        databaseName,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Skipf("local MongoDB is not available: %v", err)
	}
	defer func() {
		_ = db.RawDatabase().Drop(context.Background())
		_ = db.Close(context.Background())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, db.EnsureIndexes(ctx))

	authorID := primitive.NewObjectID()
	_, err = db.RawDatabase().Collection("users").InsertOne(ctx, bson.M{
		"_id": authorID, "username": "translation-author", "email": "translation@example.test",
	})
	require.NoError(t, err)

	recipeID := primitive.NewObjectID()
	recipe := models.Recipe{
		Id:           &recipeID,
		Author:       &models.UserView{Id: &authorID},
		Title:        "Version one",
		SourceLocale: "en",
		SourceHash:   "hash-v1",
		Ingredients:  []models.Ingredient{{Name: "flour"}},
		Steps:        []models.Step{{Description: "mix"}},
	}

	first, err := db.AddLocaleRecipe(recipe, "de")
	require.NoError(t, err)
	assert.Equal(t, "Version one", first.Title)

	translations := db.RawDatabase().Collection("recipe_translations")
	filter := bson.M{"recipe_id": recipeID, "locale": "de"}
	var firstDoc struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	require.NoError(t, translations.FindOne(ctx, filter).Decode(&firstDoc))

	recipe.Title = "Version two"
	recipe.SourceHash = "hash-v2"
	second, err := db.AddLocaleRecipe(recipe, "de")
	require.NoError(t, err)
	assert.Equal(t, "Version two", second.Title)

	reloaded, err := db.GetRecipeByIdLocale(recipeID.Hex(), "de")
	require.NoError(t, err)
	assert.Equal(t, "Version two", reloaded.Title)

	var secondDoc struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	require.NoError(t, translations.FindOne(ctx, filter).Decode(&secondDoc))
	assert.Equal(t, firstDoc.ID, secondDoc.ID, "translation _id must stay stable across replace")

	count, err := translations.CountDocuments(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
