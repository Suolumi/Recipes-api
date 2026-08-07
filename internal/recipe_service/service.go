package recipe_service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/text/language"

	"recipes/internal/database"
	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/images_manager"
	"recipes/internal/models"
	"recipes/internal/translator"
)

const (
	maxTitleLength           = 120
	maxDescriptionLength     = 5000
	maxIngredientFieldLength = 200
	maxStepTitleLength       = 200
	maxStepDescriptionLength = 5000
)

var (
	ErrInvalid  = errors.New("invalid recipe")
	ErrNotFound = errors.New("recipe not found")
)

type PictureUpload struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

type Service struct {
	db              database.Database
	translator      *translator.Translator
	imageDir        string
	stagingDir      string
	maxPictureBytes int
}

func New(db database.Database, translator *translator.Translator, imageDir string, maxPictureBytes int) (*Service, error) {
	stagingDir := filepath.Join(imageDir, ".staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			_ = os.Remove(filepath.Join(stagingDir, entry.Name()))
		}
	}
	return &Service{db: db, translator: translator, imageDir: imageDir, stagingDir: stagingDir, maxPictureBytes: maxPictureBytes}, nil
}

func normalizeLocale(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	tag, err := language.Parse(value)
	if err != nil || tag == language.Und {
		return "", fmt.Errorf("%w: invalid locale", ErrInvalid)
	}
	return tag.String(), nil
}

func validateRecipe(recipe *models.RecipeDB) error {
	recipe.Title = strings.TrimSpace(recipe.Title)
	recipe.Description = strings.TrimSpace(recipe.Description)
	if recipe.Title == "" || len([]rune(recipe.Title)) > maxTitleLength {
		return fmt.Errorf("%w: title is required and must not exceed %d characters", ErrInvalid, maxTitleLength)
	}
	if len([]rune(recipe.Description)) > maxDescriptionLength {
		return fmt.Errorf("%w: description must not exceed %d characters", ErrInvalid, maxDescriptionLength)
	}
	if recipe.Quantity < 1 {
		return fmt.Errorf("%w: quantity must be at least 1", ErrInvalid)
	}
	if !slices.Contains(models.RecipeKinds, recipe.Kind) {
		return fmt.Errorf("%w: invalid recipe kind", ErrInvalid)
	}
	if recipe.PreparationTime < 0 || recipe.CookingTime < 0 || recipe.RestingTime < 0 {
		return fmt.Errorf("%w: recipe times cannot be negative", ErrInvalid)
	}
	if len(recipe.Ingredients) == 0 {
		return fmt.Errorf("%w: at least one ingredient is required", ErrInvalid)
	}
	for i := range recipe.Ingredients {
		ingredient := &recipe.Ingredients[i]
		ingredient.Name = strings.TrimSpace(ingredient.Name)
		ingredient.Unit = strings.TrimSpace(ingredient.Unit)
		if ingredient.Name == "" || len([]rune(ingredient.Name)) > maxIngredientFieldLength || len([]rune(ingredient.Unit)) > maxIngredientFieldLength {
			return fmt.Errorf("%w: invalid ingredient at index %d", ErrInvalid, i)
		}
		if ingredient.Quantity < 0 {
			return fmt.Errorf("%w: ingredient quantity cannot be negative", ErrInvalid)
		}
	}
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("%w: at least one step is required", ErrInvalid)
	}
	for i := range recipe.Steps {
		step := &recipe.Steps[i]
		step.Title = strings.TrimSpace(step.Title)
		step.Description = strings.TrimSpace(step.Description)
		if step.Description == "" || len([]rune(step.Title)) > maxStepTitleLength || len([]rune(step.Description)) > maxStepDescriptionLength {
			return fmt.Errorf("%w: invalid step at index %d", ErrInvalid, i)
		}
	}
	locale, err := normalizeLocale(recipe.SourceLocale)
	if err != nil {
		return err
	}
	recipe.SourceLocale = locale
	return nil
}

func sourceHash(recipe models.RecipeDB) string {
	recipe.Id = nil
	recipe.Author = nil
	recipe.Locale = ""
	recipe.SourceHash = ""
	data, _ := json.Marshal(recipe)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

type stagedPicture struct {
	filename string
	path     string
}

func (s *Service) stagePictures(pictures []PictureUpload) ([]stagedPicture, error) {
	total := 0
	staged := make([]stagedPicture, 0, len(pictures))
	cleanup := func() {
		for _, picture := range staged {
			_ = os.Remove(picture.path)
		}
	}
	for _, picture := range pictures {
		total += len(picture.Data)
		if total > s.maxPictureBytes {
			cleanup()
			return nil, fmt.Errorf("%w: pictures exceed the %d byte request limit", ErrInvalid, s.maxPictureBytes)
		}
		normalized, err := images_manager.NormalizeRecipeImage(picture.Data, picture.Filename, picture.MediaType)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		filename := primitive.NewObjectID().Hex() + normalized.Extension
		path := filepath.Join(s.stagingDir, filename)
		if err := images_manager.SaveBytes(s.stagingDir, filename, normalized.Data); err != nil {
			cleanup()
			return nil, err
		}
		staged = append(staged, stagedPicture{filename: filename, path: path})
	}
	return staged, nil
}

func cleanupStaged(staged []stagedPicture) {
	for _, picture := range staged {
		_ = os.Remove(picture.path)
	}
}

func (s *Service) publish(staged []stagedPicture) error {
	for i, picture := range staged {
		if err := os.Rename(picture.path, filepath.Join(s.imageDir, picture.filename)); err != nil {
			for _, published := range staged[:i] {
				_ = images_manager.Remove(s.imageDir, published.filename)
			}
			cleanupStaged(staged[i:])
			return err
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, authorID string, input models.CreateRecipe, pictures []PictureUpload) (models.Recipe, error) {
	locale, err := normalizeLocale(input.SourceLocale)
	if err != nil {
		return models.Recipe{}, err
	}
	input.SourceLocale = locale
	recipeDB := models.RecipeDB{
		Title: input.Title, Description: input.Description, Quantity: input.Quantity, Kind: input.Kind,
		PreparationTime: input.PreparationTime, CookingTime: input.CookingTime, RestingTime: input.RestingTime,
		Ingredients: input.Ingredients, Steps: input.Steps, SourceLocale: input.SourceLocale,
	}
	if recipeDB.SourceLocale == "" && s.translator != nil {
		detected, detectErr := s.translator.GetRecipeLocale(models.Recipe{Ingredients: recipeDB.Ingredients})
		if detectErr == nil {
			recipeDB.SourceLocale, _ = normalizeLocale(detected)
		}
	}
	if err := validateRecipe(&recipeDB); err != nil {
		return models.Recipe{}, err
	}
	staged, err := s.stagePictures(pictures)
	if err != nil {
		return models.Recipe{}, err
	}
	for _, picture := range staged {
		recipeDB.Pictures = append(recipeDB.Pictures, picture.filename)
	}
	recipeDB.SourceHash = sourceHash(recipeDB)
	create := models.CreateRecipe{
		Title: recipeDB.Title, Description: recipeDB.Description, Quantity: recipeDB.Quantity, Kind: recipeDB.Kind,
		PreparationTime: recipeDB.PreparationTime, CookingTime: recipeDB.CookingTime, RestingTime: recipeDB.RestingTime,
		Ingredients: recipeDB.Ingredients, Steps: recipeDB.Steps, Pictures: recipeDB.Pictures,
		SourceLocale: recipeDB.SourceLocale, SourceHash: recipeDB.SourceHash,
	}
	created, err := s.db.CreateRecipe(authorID, &create)
	if err != nil {
		cleanupStaged(staged)
		return models.Recipe{}, err
	}
	if err := s.publish(staged); err != nil {
		_, _ = s.db.DeleteRecipeById(created.Id.Hex())
		return models.Recipe{}, err
	}
	created.Locale = created.SourceLocale
	return created, nil
}

func mergePatch(recipe models.Recipe, patch models.UpdateRecipeRequest) (models.RecipeDB, error) {
	merged := recipe.ToRecipeDB()
	merged.Locale = ""
	if patch.Title != nil {
		merged.Title = *patch.Title
	}
	if patch.Description != nil {
		merged.Description = *patch.Description
	}
	if patch.Quantity != nil {
		merged.Quantity = *patch.Quantity
	}
	if patch.Kind != nil {
		merged.Kind = *patch.Kind
	}
	if patch.PreparationTime != nil {
		merged.PreparationTime = *patch.PreparationTime
	}
	if patch.CookingTime != nil {
		merged.CookingTime = *patch.CookingTime
	}
	if patch.RestingTime != nil {
		merged.RestingTime = *patch.RestingTime
	}
	if patch.Ingredients != nil {
		merged.Ingredients = slices.Clone(*patch.Ingredients)
	}
	if patch.Steps != nil {
		merged.Steps = slices.Clone(*patch.Steps)
	}
	if patch.Locale != nil {
		merged.SourceLocale = *patch.Locale
	}
	if patch.KeepPictureIDs != nil {
		available := make(map[string]bool, len(recipe.Pictures))
		for _, id := range recipe.Pictures {
			available[id] = true
		}
		seen := make(map[string]bool, len(*patch.KeepPictureIDs))
		merged.Pictures = nil
		for _, id := range *patch.KeepPictureIDs {
			if !available[id] || seen[id] {
				return models.RecipeDB{}, fmt.Errorf("%w: invalid or duplicate keep_picture_ids entry", ErrInvalid)
			}
			seen[id] = true
			merged.Pictures = append(merged.Pictures, id)
		}
	}
	return merged, nil
}

func (s *Service) Update(ctx context.Context, recipe models.Recipe, patch models.UpdateRecipeRequest, pictures []PictureUpload) (models.Recipe, error) {
	original := recipe.ToRecipeDB()
	original.Locale = ""
	merged, err := mergePatch(recipe, patch)
	if err != nil {
		return models.Recipe{}, err
	}
	staged, err := s.stagePictures(pictures)
	if err != nil {
		return models.Recipe{}, err
	}
	for _, picture := range staged {
		merged.Pictures = append(merged.Pictures, picture.filename)
	}
	if err := validateRecipe(&merged); err != nil {
		cleanupStaged(staged)
		return models.Recipe{}, err
	}
	merged.SourceHash = sourceHash(merged)
	updated, err := s.db.ReplaceRecipeById(ctx, recipe.Id.Hex(), merged)
	if err != nil {
		cleanupStaged(staged)
		return models.Recipe{}, err
	}
	if err := s.publish(staged); err != nil {
		_, _ = s.db.ReplaceRecipeById(ctx, recipe.Id.Hex(), original)
		return models.Recipe{}, err
	}
	kept := make(map[string]bool, len(updated.Pictures))
	for _, id := range updated.Pictures {
		kept[id] = true
	}
	for _, id := range recipe.Pictures {
		if !kept[id] {
			_ = images_manager.Remove(s.imageDir, id)
		}
	}
	_ = s.db.DeleteLocalizedRecipesByID(ctx, recipe.Id.Hex())
	updated.Locale = updated.SourceLocale
	return updated, nil
}

func (s *Service) GetForUser(ctx context.Context, recipeID, userID, locale string) (models.Recipe, error) {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return models.Recipe{}, ErrNotFound
	}
	locale, err := normalizeLocale(locale)
	if err != nil {
		return models.Recipe{}, err
	}
	recipe, err := s.db.GetRecipeByIdForAuthor(ctx, recipeID, userID)
	if err != nil {
		if errors.Is(err, mongorepo.NotFoundError) {
			return models.Recipe{}, ErrNotFound
		}
		return models.Recipe{}, err
	}
	return s.localize(ctx, recipe, locale), nil
}

func (s *Service) Get(ctx context.Context, recipeID, locale string) (models.Recipe, error) {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return models.Recipe{}, ErrNotFound
	}
	locale, err := normalizeLocale(locale)
	if err != nil {
		return models.Recipe{}, err
	}
	recipe, err := s.db.GetRecipeById(recipeID)
	if err != nil {
		if errors.Is(err, mongorepo.NotFoundError) {
			return models.Recipe{}, ErrNotFound
		}
		return models.Recipe{}, err
	}
	return s.localize(ctx, recipe, locale), nil
}

func sameBaseLocale(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	l, _ := language.Parse(left)
	r, _ := language.Parse(right)
	lb, _ := l.Base()
	rb, _ := r.Base()
	return lb == rb
}

func (s *Service) localize(ctx context.Context, canonical models.Recipe, requested string) models.Recipe {
	needsBackfill := canonical.SourceLocale == "" || canonical.SourceHash == ""
	if canonical.SourceLocale == "" && s.translator != nil {
		if detected, err := s.translator.GetRecipeLocale(canonical); err == nil {
			canonical.SourceLocale, _ = normalizeLocale(detected)
		}
	}
	if canonical.SourceHash == "" {
		canonical.SourceHash = sourceHash(canonical.ToRecipeDB())
	}
	if needsBackfill && canonical.SourceLocale != "" && canonical.Id != nil {
		// Backfill locale/hash for recipes created before localization metadata existed.
		if stored, err := s.db.ReplaceRecipeById(ctx, canonical.Id.Hex(), canonical.ToRecipeDB()); err == nil {
			canonical = stored
		}
	}
	requested, err := normalizeLocale(requested)
	if err != nil || requested == "" || sameBaseLocale(canonical.SourceLocale, requested) {
		canonical.Locale = canonical.SourceLocale
		return canonical
	}
	localized, err := s.db.GetRecipeByIdLocale(canonical.Id.Hex(), requested)
	if err == nil && localized.SourceHash != "" && localized.SourceHash == canonical.SourceHash {
		localized.Locale = requested
		return localized
	}
	if s.translator == nil {
		canonical.Locale = canonical.SourceLocale
		return canonical
	}
	translated, err := s.translator.TranslateRecipe(canonical, requested)
	if err != nil {
		canonical.Locale = canonical.SourceLocale
		return canonical
	}
	translated.SourceLocale = canonical.SourceLocale
	translated.Locale = requested
	translated.SourceHash = canonical.SourceHash
	stored, err := s.db.AddLocaleRecipe(translated, requested)
	if err != nil {
		return translated
	}
	stored.Locale = requested
	return stored
}

func (s *Service) List(ctx context.Context, parameters models.GetRecipesRequest) ([]models.RecipePreview, int64, error) {
	locale, err := normalizeLocale(parameters.Locale)
	if err != nil {
		return nil, 0, err
	}
	searchLocale, err := normalizeLocale(parameters.SearchLocale)
	if err != nil {
		return nil, 0, err
	}
	parameters.Locale = locale
	parameters.SearchLocale = searchLocale
	documents, count, err := s.db.GetRecipeDocuments(parameters)
	if err != nil {
		return nil, 0, err
	}
	return s.localizePreviews(ctx, documents, parameters.Locale), count, nil
}

func (s *Service) ListForUser(ctx context.Context, userID, cursor string, limit int, locale string) ([]models.RecipePreview, int64, string, error) {
	locale, err := normalizeLocale(locale)
	if err != nil {
		return nil, 0, "", err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	documents, count, err := s.db.GetRecipesByAuthor(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, 0, "", err
	}
	next := ""
	if len(documents) > limit {
		next = documents[limit-1].Id.Hex()
		documents = documents[:limit]
	}
	return s.localizePreviews(ctx, documents, locale), count, next, nil
}

func (s *Service) localizePreviews(ctx context.Context, documents []models.Recipe, locale string) []models.RecipePreview {
	previews := make([]models.RecipePreview, 0, len(documents))
	for _, document := range documents {
		recipe := s.localize(ctx, document, locale)
		previews = append(previews, models.RecipePreview{
			Id: recipe.Id, Title: recipe.Title, Description: recipe.Description, Author: recipe.Author,
			PreparationTime: recipe.PreparationTime, CookingTime: recipe.CookingTime, RestingTime: recipe.RestingTime,
			Kind: recipe.Kind, Quantity: recipe.Quantity, Pictures: recipe.Pictures,
			SourceLocale: recipe.SourceLocale, Locale: recipe.Locale,
		})
	}
	return previews
}

func (s *Service) Delete(ctx context.Context, recipeID string) (models.RecipeDB, error) {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return models.RecipeDB{}, ErrNotFound
	}
	deleted, err := s.db.DeleteRecipeById(recipeID)
	if err != nil {
		if errors.Is(err, mongorepo.NotFoundError) {
			return models.RecipeDB{}, ErrNotFound
		}
		return models.RecipeDB{}, err
	}
	for _, picture := range deleted.Pictures {
		_ = images_manager.Remove(s.imageDir, picture)
	}
	_ = s.db.DeleteLocalizedRecipesByID(ctx, recipeID)
	return deleted, nil
}
