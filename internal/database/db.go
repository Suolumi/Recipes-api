package database

import "recipes/internal/models"

type Database interface {
	CreateUser(user models.UserDB) (string, error)
	GetUsers(username string, limit, offset int) ([]models.UserDB, int64, error)
	GetUserById(id string) (models.UserDB, error)
	UpdateUserById(id string, user models.UserDB) (models.UserDB, error)
	UpdateUserInterfaceById(id string, user interface{}) (models.UserDB, error)
	DeleteUserById(id string) (models.UserDB, error)
	UserConflicts(user models.UserDB) (models.UserDB, error)

	CreateRecipe(authorId string, infos *models.CreateRecipe) (models.Recipe, error)
	GetRecipes(parameters models.GetRecipesRequest) ([]models.RecipePreview, int64, error)
	GetRecipeById(id string) (models.Recipe, error)
	UpdateRecipeById(id string, recipe *models.UpdateRecipeRequest) (models.Recipe, error)
	DeleteRecipeById(id string) (models.RecipeDB, error)
	RecipeConflicts(recipe models.RecipeDB) (models.RecipeDB, error)
}
