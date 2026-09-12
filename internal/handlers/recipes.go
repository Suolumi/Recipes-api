package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"recipes/internal/jwt_manager"
	"recipes/internal/models"
	"recipes/internal/recipe_service"
	"recipes/internal/utils"
)

const maxRecipeRequestBytes = 90 << 20

func recipeServiceError(err error, c echo.Context) error {
	switch {
	case errors.Is(err, recipe_service.ErrInvalid):
		return errorResponse(http.StatusUnprocessableEntity, err.Error(), nil, c)
	case errors.Is(err, recipe_service.ErrNotFound):
		return errorResponse(http.StatusNotFound, "Recipe not found", nil, c)
	default:
		return errorResponse(http.StatusInternalServerError, "Could not process recipe", err, c)
	}
}

func readPictureParts(files []*multipart.FileHeader) ([]recipe_service.PictureUpload, error) {
	pictures := make([]recipe_service.PictureUpload, 0, len(files))
	total := 0
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxRecipeRequestBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += len(data)
		if total > maxRecipeRequestBytes {
			return nil, errors.New("pictures exceed the request size limit")
		}
		pictures = append(pictures, recipe_service.PictureUpload{
			Filename: header.Filename, MediaType: header.Header.Get(echo.HeaderContentType), Data: data,
		})
	}
	return pictures, nil
}

func bindRecipeRequest(c echo.Context, target any) ([]recipe_service.PictureUpload, error) {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusUnsupportedMediaType, "missing or invalid content type")
	}
	switch mediaType {
	case echo.MIMEApplicationJSON:
		decoder := json.NewDecoder(io.LimitReader(c.Request().Body, maxRecipeRequestBytes+1))
		if err := decoder.Decode(target); err != nil {
			return nil, err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, errors.New("request body must contain one JSON object")
		}
		return nil, nil
	case echo.MIMEMultipartForm:
		if err := c.Request().ParseMultipartForm(maxRecipeRequestBytes); err != nil {
			return nil, err
		}
		defer c.Request().MultipartForm.RemoveAll()
		recipeJSON := c.FormValue("recipe")
		if strings.TrimSpace(recipeJSON) == "" {
			return nil, errors.New("multipart field recipe is required")
		}
		decoder := json.NewDecoder(strings.NewReader(recipeJSON))
		if err := decoder.Decode(target); err != nil {
			return nil, err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, errors.New("recipe field must contain one JSON object")
		}
		return readPictureParts(c.Request().MultipartForm.File["pictures"])
	default:
		return nil, echo.NewHTTPError(http.StatusUnsupportedMediaType, "use application/json or multipart/form-data")
	}
}

// CreateRecipe creates a complete recipe. Multipart requests contain a JSON
// `recipe` field and zero or more `pictures` file fields.
func (h *Handlers) CreateRecipe(c echo.Context) error {
	var body models.CreateRecipe
	pictures, err := bindRecipeRequest(c, &body)
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			message, ok := httpErr.Message.(string)
			if !ok {
				message = http.StatusText(httpErr.Code)
			}
			return errorResponse(httpErr.Code, message, nil, c)
		}
		return errorResponse(http.StatusBadRequest, err.Error(), nil, c)
	}
	jwt := jwt_manager.GetJwt[*models.TokenClaims](c)
	recipe, err := h.recipes.Create(c.Request().Context(), jwt.UserId, body, pictures)
	if err != nil {
		return recipeServiceError(err, c)
	}
	return c.JSON(http.StatusCreated, recipe)
}

func (h *Handlers) GetRecipes(c echo.Context) error {
	for _, name := range []string{"locale", "search_locale"} {
		if len(c.QueryParams()[name]) > 1 {
			return errorResponse(http.StatusBadRequest, name+" may be specified only once", nil, c)
		}
	}
	var body models.GetRecipesRequest
	if err := utils.BindQuery(c, &body); err != nil {
		return errorResponse(http.StatusBadRequest, err.Error(), nil, c)
	}
	if body.Limit == 0 {
		body.Limit = 10
	}
	if body.Limit < 0 || body.Limit > 100 || body.Offset < 0 {
		return errorResponse(http.StatusBadRequest, "limit must be between 0 and 100 and offset must not be negative", nil, c)
	}
	recipes, count, err := h.recipes.List(c.Request().Context(), body)
	if err != nil {
		return recipeServiceError(err, c)
	}
	if recipes == nil {
		recipes = []models.RecipePreview{}
	}
	return c.JSON(http.StatusOK, models.GetRecipesResponse{Length: count, Items: recipes})
}

func (h *Handlers) GetRecipe(c echo.Context) error {
	if len(c.QueryParams()["locale"]) > 1 {
		return errorResponse(http.StatusBadRequest, "locale may be specified only once", nil, c)
	}
	recipe, err := h.recipes.Get(c.Request().Context(), c.Param("id"), c.QueryParam("locale"))
	if err != nil {
		return recipeServiceError(err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

func (h *Handlers) UpdateRecipe(c echo.Context) error {
	var body models.UpdateRecipeRequest
	pictures, err := bindRecipeRequest(c, &body)
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			message, _ := httpErr.Message.(string)
			return errorResponse(httpErr.Code, message, nil, c)
		}
		return errorResponse(http.StatusBadRequest, err.Error(), nil, c)
	}
	recipe := c.Get("recipe").(models.Recipe)
	updated, err := h.recipes.Update(c.Request().Context(), recipe, body, pictures)
	if err != nil {
		return recipeServiceError(err, c)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *Handlers) DeleteRecipe(c echo.Context) error {
	recipe, err := h.recipes.Delete(c.Request().Context(), c.Param("id"))
	if err != nil {
		return recipeServiceError(err, c)
	}
	return c.JSON(http.StatusOK, recipe)
}

// RetranslateRecipe schedules a fresh translation of one recipe into every
// configured target locale. Admin only; returns once the work is queued.
func (h *Handlers) RetranslateRecipe(c echo.Context) error {
	if err := h.recipes.Retranslate(c.Request().Context(), c.Param("id")); err != nil {
		return recipeServiceError(err, c)
	}
	return messageResponse(http.StatusAccepted, "Retranslation scheduled", c)
}

func (h *Handlers) RecipeAuthorMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		jwt := jwt_manager.GetJwt[*models.TokenClaims](c)
		recipe, err := h.db.GetRecipeById(c.Param("id"))
		if err != nil {
			return errorResponse(http.StatusNotFound, "Recipe not found", nil, c)
		}
		c.Set("recipe", recipe)
		if recipe.Author.Id.Hex() == jwt.UserId || jwt.Admin {
			return next(c)
		}
		return errorResponse(http.StatusUnauthorized, "Unauthorized", nil, c)
	}
}
