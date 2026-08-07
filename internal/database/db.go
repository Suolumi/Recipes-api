package database

import (
	"context"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"recipes/internal/models"
)

type Database interface {
	EnsureIndexes(ctx context.Context) error
	MigrateLegacyTranslations(ctx context.Context) error
	Close(ctx context.Context) error
	CreateUser(user models.UserDB) (models.UserDB, error)
	GetUsers(username string, limit, offset int) ([]models.UserDB, int64, error)
	GetUserById(id string) (models.UserDB, error)
	GetUserByIdentifier(identifier string) (models.UserDB, error)
	UpdateUserById(id string, user models.UserDB) (models.UserDB, error)
	UpdateUserInterfaceById(id string, user interface{}) (models.UserDB, error)
	DeleteUserById(id string) (models.UserDB, error)
	UserConflicts(user models.UserDB) (models.UserDB, error)

	CreateRecipe(authorId string, infos *models.CreateRecipe) (models.Recipe, error)
	AddLocaleRecipe(recipe models.Recipe, locale string) (models.Recipe, error)
	GetRecipes(parameters models.GetRecipesRequest) ([]models.RecipePreview, int64, error)
	GetRecipeDocuments(parameters models.GetRecipesRequest) ([]models.Recipe, int64, error)
	GetRecipesByAuthor(ctx context.Context, authorID, cursor string, limit int) ([]models.Recipe, int64, error)
	GetRecipeById(id string) (models.Recipe, error)
	GetRecipeByIdForAuthor(ctx context.Context, id, authorID string) (models.Recipe, error)
	GetRecipeByIdLocale(id string, locale string) (models.Recipe, error)
	GetLocaleRecipesByIDs(ctx context.Context, ids []string, locale string) ([]models.Recipe, error)
	ReferencedPictures(ctx context.Context) (map[string]struct{}, error)
	UpdateRecipeById(id string, recipe *models.UpdateRecipeRequest) (models.Recipe, error)
	ReplaceRecipeById(ctx context.Context, id string, recipe models.RecipeDB) (models.Recipe, error)
	DeleteRecipeById(id string) (models.RecipeDB, error)
	DeleteLocalizedRecipesByID(ctx context.Context, id string) error
	RecipeConflicts(recipe models.RecipeDB) (models.RecipeDB, error)
	RawDatabase() *mongodriver.Database
}
