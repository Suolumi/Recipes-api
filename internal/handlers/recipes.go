package handlers

import (
	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slices"
	"net/http"
	"recipes/internal/images_manager"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
	"recipes/internal/utils"
)

// @Summary Create Recipe
// @Description Create a recipe with required information
// @Tags Recipes
// @accept json
// @produce json
// @Param request body models.CreateRecipe true "Recipe information"
// @Success 200 {object} models.Recipe "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 500 {object} models.ErrorResponse "Failed to create the recipe"
// @Security BearerAuth
// @Router /recipes [post]
func (h *Handlers) CreateRecipe(c echo.Context) error {
	var body models.CreateRecipe
	jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
	err := c.Bind(&body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}
	recipe, err := h.db.CreateRecipe(jwt.UserId, &body)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	for _, k := range recipe.Pictures {
		h.imgLru.Add(k, false)
		h.imgLru.Remove(k)
	}
	return c.JSON(http.StatusCreated, recipe)
}

// @Summary Get Recipes (preview)
// @Description Get multiple recipe previews with query parameters
// @Tags Recipes
// @accept json
// @produce json
// @Param request query models.GetRecipesRequest false "Query params"
// @Success 200 {object} models.GetRecipesResponse "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 500 {object} models.ErrorResponse "Failed to get the recipes"
// @Security BearerAuth
// @Router /recipes [get]
func (h *Handlers) GetRecipes(c echo.Context) error {
	var body models.GetRecipesRequest
	err := utils.BindQuery(c, &body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}
	// Limit default payload size
	if body.Limit == 0 {
		body.Limit = 10
	}
	recipes, nbRecipes, err := h.db.GetRecipes(body)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	if recipes == nil {
		recipes = []models.RecipePreview{}
	}
	return c.JSON(http.StatusOK, models.GetRecipesResponse{
		Length: nbRecipes,
		Items:  recipes,
	})
}

// @Summary Get Recipe
// @Description Get the full information of a recipe
// @Tags Recipes
// @accept json
// @produce json
// @Success 200 {object} models.Recipe "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 403 {object} models.ErrorResponse "Unauthorized"
// @Failure 404 {object} models.ErrorResponse "Recipe not found"
// @Security BearerAuth
// @Router /recipes/:id [get]
func (h *Handlers) GetRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	recipe, err := h.db.GetRecipeById(recipeId)
	if err != nil {
		return errorResponse(http.StatusNotFound, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

// @Summary Update Recipe
// @Description Update a recipe with information
// @Tags Recipes
// @accept json
// @produce json
// @Param request body models.UpdateRecipeRequest true "Recipe information"
// @Success 200 {object} models.Recipe "OK"
// @Failure 400 {object} models.ErrorResponse "Bad Request"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 403 {object} models.ErrorResponse "Unauthorized"
// @Failure 404 {object} models.ErrorResponse "Recipe not found"
// @Failure 500 {object} models.ErrorResponse "Failed to update the recipe"
// @Security BearerAuth
// @Router /recipes/:id [patch]
func (h *Handlers) UpdateRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	var body models.UpdateRecipeRequest
	err := c.Bind(&body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	if len(body.Pictures) > 0 {
		recipe := c.Get("recipe").(models.Recipe)
		addPictures := slices.Clone(body.Pictures)
		slices.DeleteFunc(addPictures, func(s string) bool {
			length := len(recipe.Pictures)
			recipe.Pictures = slices.DeleteFunc(recipe.Pictures, func(s2 string) bool { return s2 == s })
			return length != len(recipe.Pictures)
		})

		for _, deletePicture := range recipe.Pictures {
			err := images_manager.Remove(h.cfg.RecipeImageDir, deletePicture)
			if err != nil {
				return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
			}
		}
		for _, addPicture := range addPictures {
			h.imgLru.Add(addPicture, false)
			h.imgLru.Remove(addPicture)
		}
	}

	updated, err := h.db.UpdateRecipeById(recipeId, &body)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, updated)
}

// @Summary Delete Recipe
// @Description Delete a recipe
// @Tags Recipes
// @accept json
// @produce json
// @Success 200 {object} models.Recipe "OK"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired jwt"
// @Failure 403 {object} models.ErrorResponse "Unauthorized"
// @Failure 404 {object} models.ErrorResponse "Recipe not found"
// @Failure 500 {object} models.ErrorResponse "Failed to delete the recipe"
// @Security BearerAuth
// @Router /recipes/:id [delete]
func (h *Handlers) DeleteRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	recipe, err := h.db.DeleteRecipeById(recipeId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	for _, img := range recipe.Pictures {
		tmpErr := images_manager.Remove(h.cfg.RecipeImageDir, img)
		if tmpErr != nil {
			err = tmpErr
		}
	}
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

func (h *Handlers) RecipeAuthorMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
		recipeId := c.Param("id")
		recipe, err := h.db.GetRecipeById(recipeId)
		if err != nil {
			return errorResponse(http.StatusNotFound, err.Error(), err, c)
		}

		c.Set("recipe", recipe)
		if recipe.Author.Id.Hex() == jwt.UserId || jwt.Admin {
			return next(c)
		}
		return errorResponse(http.StatusUnauthorized, "Unauthorized", nil, c)
	}
}
