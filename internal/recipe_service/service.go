package recipe_service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/text/language"

	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/images_manager"
	"recipes/internal/models"
	"recipes/internal/utils"
)

const (
	maxTitleLength           = 120
	maxDescriptionLength     = 5000
	maxIngredientFieldLength = 200
	maxStepTitleLength       = 200
	maxStepDescriptionLength = 5000

	// translateConcurrency caps how many recipes are being translated at once
	// across all detached write-path work.
	translateConcurrency = 4
)

// translateBackoffs is the wait before each attempt when translating one locale;
// its length is the attempt count.
var translateBackoffs = []time.Duration{time.Second, 4 * time.Second, 10 * time.Second}

var (
	ErrInvalid  = errors.New("invalid recipe")
	ErrNotFound = errors.New("recipe not found")
)

type PictureUpload struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

// Store is the slice of database.Database the service needs; database.Database
// satisfies it. Keeping it narrow lets tests fake the parts under exercise.
type Store interface {
	CreateRecipe(authorID string, infos *models.CreateRecipe) (models.Recipe, error)
	ReplaceRecipeById(ctx context.Context, id string, recipe models.RecipeDB) (models.Recipe, error)
	GetRecipeById(id string) (models.Recipe, error)
	GetRecipeByIdForAuthor(ctx context.Context, id, authorID string) (models.Recipe, error)
	GetRecipeByIdLocale(id string, locale string) (models.Recipe, error)
	GetRecipeDocuments(parameters models.GetRecipesRequest) ([]models.Recipe, int64, error)
	GetRecipesByAuthor(ctx context.Context, authorID, cursor string, limit int) ([]models.Recipe, int64, error)
	GetTranslationsByRecipeIDs(ctx context.Context, ids []string, locale string) (map[string]models.Recipe, error)
	AddLocaleRecipe(recipe models.Recipe, locale string) (models.Recipe, error)
	DeleteRecipeById(id string) (models.RecipeDB, error)
	DeleteLocalizedRecipesByID(ctx context.Context, id string) error
}

// Translator is the slice of *translator.Translator the service needs. A nil
// Translator disables source-locale detection and translate-on-write.
type Translator interface {
	TranslateRecipe(recipe models.Recipe, to string) (models.Recipe, error)
	GetRecipeLocale(recipe models.Recipe) (string, error)
}

type Service struct {
	db              Store
	translator      Translator
	imageDir        string
	maxPictureBytes int
	targetLocales   []string
	translateSlots  chan struct{}
}

func New(db Store, translator Translator, imageDir string, maxPictureBytes int, targetLocales []string) (*Service, error) {
	return &Service{
		db:              db,
		translator:      translator,
		imageDir:        imageDir,
		maxPictureBytes: maxPictureBytes,
		targetLocales:   targetLocales,
		translateSlots:  make(chan struct{}, translateConcurrency),
	}, nil
}

// canTranslate reports whether translate-on-write is configured.
func (s *Service) canTranslate() bool {
	return s.translator != nil && len(s.targetLocales) > 0
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

// normalizedPicture is an uploaded image after in-memory normalization, with the
// final filename it will take in the image directory. Nothing is written to disk
// until writePictures runs.
type normalizedPicture struct {
	filename string
	data     []byte
}

func (s *Service) normalizePictures(pictures []PictureUpload) ([]normalizedPicture, error) {
	total := 0
	normalized := make([]normalizedPicture, 0, len(pictures))
	for _, picture := range pictures {
		total += len(picture.Data)
		if total > s.maxPictureBytes {
			return nil, fmt.Errorf("%w: pictures exceed the %d byte request limit", ErrInvalid, s.maxPictureBytes)
		}
		image, err := images_manager.NormalizeRecipeImage(picture.Data, picture.Filename, picture.MediaType)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		normalized = append(normalized, normalizedPicture{
			filename: primitive.NewObjectID().Hex() + image.Extension,
			data:     image.Data,
		})
	}
	return normalized, nil
}

// writePictures writes the normalized bytes into the image directory. On the
// first failure it removes what it already wrote and returns the error.
func (s *Service) writePictures(pictures []normalizedPicture) ([]string, error) {
	written := make([]string, 0, len(pictures))
	for _, picture := range pictures {
		if err := images_manager.SaveBytes(s.imageDir, picture.filename, picture.data); err != nil {
			s.removePictures(written)
			return nil, err
		}
		written = append(written, picture.filename)
	}
	return written, nil
}

func (s *Service) removePictures(filenames []string) {
	for _, filename := range filenames {
		_ = images_manager.Remove(s.imageDir, filename)
	}
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
	normalized, err := s.normalizePictures(pictures)
	if err != nil {
		return models.Recipe{}, err
	}
	for _, picture := range normalized {
		recipeDB.Pictures = append(recipeDB.Pictures, picture.filename)
	}
	recipeDB.SourceHash = sourceHash(recipeDB)
	written, err := s.writePictures(normalized)
	if err != nil {
		return models.Recipe{}, err
	}
	create := utils.DupStruct[models.CreateRecipe](&recipeDB)
	created, err := s.db.CreateRecipe(authorID, &create)
	if err != nil {
		s.removePictures(written)
		return models.Recipe{}, err
	}
	s.scheduleTranslations(created)
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
	merged, err := mergePatch(recipe, patch)
	if err != nil {
		return models.Recipe{}, err
	}
	normalized, err := s.normalizePictures(pictures)
	if err != nil {
		return models.Recipe{}, err
	}
	for _, picture := range normalized {
		merged.Pictures = append(merged.Pictures, picture.filename)
	}
	if err := validateRecipe(&merged); err != nil {
		return models.Recipe{}, err
	}
	merged.SourceHash = sourceHash(merged)
	written, err := s.writePictures(normalized)
	if err != nil {
		return models.Recipe{}, err
	}
	updated, err := s.db.ReplaceRecipeById(ctx, recipe.Id.Hex(), merged)
	if err != nil {
		s.removePictures(written)
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
	s.scheduleTranslations(updated)
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
	return s.localize(recipe, locale), nil
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
	return s.localize(recipe, locale), nil
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

// wantsTranslation reports whether a translation row is worth looking up for the
// requested locale: it must be a valid, non-empty locale that differs from the
// recipe's source, and the recipe must carry the id and hash a row is keyed on.
func wantsTranslation(canonical models.Recipe, requested string) bool {
	return requested != "" && canonical.Id != nil && canonical.SourceHash != "" &&
		!sameBaseLocale(canonical.SourceLocale, requested)
}

// pickTranslation returns the stored translation when it matches the canonical's
// source hash, otherwise the canonical recipe. Either way Locale is stamped with
// what is actually being served.
func pickTranslation(canonical, translation models.Recipe, requested string, found bool) models.Recipe {
	if found && translation.SourceHash != "" && translation.SourceHash == canonical.SourceHash {
		translation.Locale = requested
		return translation
	}
	canonical.Locale = canonical.SourceLocale
	return canonical
}

// localize serves the stored translation for one recipe, or the canonical recipe
// when none is current. It never calls the translator and never writes.
func (s *Service) localize(canonical models.Recipe, requested string) models.Recipe {
	requested, err := normalizeLocale(requested)
	if err != nil || !wantsTranslation(canonical, requested) {
		canonical.Locale = canonical.SourceLocale
		return canonical
	}
	translation, err := s.db.GetRecipeByIdLocale(canonical.Id.Hex(), requested)
	return pickTranslation(canonical, translation, requested, err == nil)
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

func (s *Service) localizePreviews(ctx context.Context, documents []models.Recipe, requested string) []models.RecipePreview {
	requested, err := normalizeLocale(requested)
	translations := map[string]models.Recipe{}
	if err == nil && requested != "" {
		ids := make([]string, 0, len(documents))
		for _, document := range documents {
			if wantsTranslation(document, requested) {
				ids = append(ids, document.Id.Hex())
			}
		}
		if len(ids) > 0 {
			if fetched, fetchErr := s.db.GetTranslationsByRecipeIDs(ctx, ids, requested); fetchErr == nil {
				translations = fetched
			}
		}
	}
	previews := make([]models.RecipePreview, 0, len(documents))
	for _, document := range documents {
		recipe := document
		recipe.Locale = document.SourceLocale
		if document.Id != nil {
			translation, found := translations[document.Id.Hex()]
			recipe = pickTranslation(document, translation, requested, found)
		}
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
