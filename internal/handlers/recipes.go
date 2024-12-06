package handlers

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"recipes/internal/images_manager"
	"recipes/internal/jwt_manager"
	"recipes/internal/models"
)

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
		_ = images_manager.Remove(h.cfg.ImagesDir, k)
		h.imgLru.Add(k, true)
		h.imgLru.Remove(k)
	}
	return c.JSON(http.StatusCreated, recipe)
}

func (h *Handlers) GetRecipes(c echo.Context) error {
	var body models.GetRecipesRequest
	err := c.Bind(&body)
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

func (h *Handlers) GetRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	recipe, err := h.db.GetRecipeById(recipeId)
	if err != nil {
		return errorResponse(http.StatusNotFound, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

func (h *Handlers) UpdateRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	var body models.UpdateRecipeRequest
	err := c.Bind(&body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), err, c)
	}

	recipe, err := h.db.UpdateRecipeById(recipeId, &body)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

func (h *Handlers) DeleteRecipe(c echo.Context) error {
	recipeId := c.Param("id")
	recipe, err := h.db.DeleteRecipeById(recipeId)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
	}
	for _, img := range recipe.Pictures {
		err = images_manager.Remove(h.cfg.RecipeImageDir, img)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, err.Error(), err, c)
		}
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

		if recipe.Author.Id.Hex() == jwt.UserId || jwt.Admin {
			return next(c)
		}
		return errorResponse(http.StatusUnauthorized, "Unauthorized", nil, c)
	}
}
