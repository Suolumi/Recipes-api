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
	ErrInvalid      = errors.New("invalid recipe")
	ErrNotFound     = errors.New("recipe not found")
	ErrHasFavorites = errors.New("recipe has favorites")
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
	GetRecipeDocumentsBoosted(ctx context.Context, userID string, parameters models.GetRecipesRequest) (favorited, rest []models.Recipe, total int64, err error)
	GetRecipesByAuthor(ctx context.Context, authorID, cursor string, limit int) ([]models.Recipe, int64, error)
	GetTranslationsByRecipeIDs(ctx context.Context, ids []string, locale string) (map[string]models.Recipe, error)
	AddLocaleRecipe(recipe models.Recipe, locale string) (models.Recipe, error)
	DeleteRecipeById(id string) (models.RecipeDB, error)
	DeleteLocalizedRecipesByID(ctx context.Context, id string) error
	GetOldestVariationID(ctx context.Context, rootID string) (*primitive.ObjectID, error)
	RepointVariations(ctx context.Context, oldRootID, newRootID string) error
	PromoteRecipeToRoot(ctx context.Context, id string) error
	GetVariationCounts(ctx context.Context, rootIDs []string) (map[string]int64, error)

	AddFavorite(ctx context.Context, userID, recipeID string) error
	RemoveFavorite(ctx context.Context, userID, recipeID string) error
	DeleteFavoritesByRecipeID(ctx context.Context, recipeID string) error
	GetFavoriteInfo(ctx context.Context, ids []string, userID string) (map[string]models.FavoriteInfo, error)
	GetFamilyFavoriteInfo(ctx context.Context, rootIDs []string, userID string) (map[string]models.FavoriteInfo, error)
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
	seenLabels := make(map[string]string)
	for i := range recipe.Ingredients {
		ingredient := &recipe.Ingredients[i]
		ingredient.Name = strings.TrimSpace(ingredient.Name)
		ingredient.Unit = strings.TrimSpace(ingredient.Unit)
		ingredient.Label = strings.TrimSpace(ingredient.Label)
		if ingredient.Name == "" || len([]rune(ingredient.Name)) > maxIngredientFieldLength ||
			len([]rune(ingredient.Unit)) > maxIngredientFieldLength || len([]rune(ingredient.Label)) > maxIngredientFieldLength {
			return fmt.Errorf("%w: invalid ingredient at index %d", ErrInvalid, i)
		}
		if ingredient.Quantity < 0 {
			return fmt.Errorf("%w: ingredient quantity cannot be negative", ErrInvalid)
		}
		if ingredient.Label != "" {
			key := strings.ToLower(ingredient.Label)
			if canonical, ok := seenLabels[key]; ok {
				ingredient.Label = canonical
			} else {
				seenLabels[key] = ingredient.Label
			}
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
	recipe.VariationOf = nil
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

// stepPictureUploads validates that every upload's step index is within
// range and returns the uploads in deterministic (ascending index) order,
// paired with which step index each belongs to.
func (s *Service) stepPictureUploads(uploads map[int]PictureUpload, stepCount int) ([]int, []PictureUpload, error) {
	if len(uploads) == 0 {
		return nil, nil, nil
	}
	indices := make([]int, 0, len(uploads))
	for i := range uploads {
		if i < 0 || i >= stepCount {
			return nil, nil, fmt.Errorf("%w: step picture index %d is out of range", ErrInvalid, i)
		}
		indices = append(indices, i)
	}
	slices.Sort(indices)
	result := make([]PictureUpload, len(indices))
	for j, i := range indices {
		result[j] = uploads[i]
	}
	return indices, result, nil
}

// normalizeAndWriteAllPictures normalizes recipe-level pictures and step
// pictures together (so s.maxPictureBytes caps their combined size) and
// writes them all in one batch, so a failure partway through cleans up
// everything already written. It returns the recipe-level filenames and,
// separately, the step index/filename pairs to assign onto steps.
func (s *Service) normalizeAndWriteAllPictures(pictures []PictureUpload, stepUploads []PictureUpload, stepIndices []int) ([]string, map[int]string, error) {
	combined := make([]PictureUpload, 0, len(pictures)+len(stepUploads))
	combined = append(combined, pictures...)
	combined = append(combined, stepUploads...)
	normalized, err := s.normalizePictures(combined)
	if err != nil {
		return nil, nil, err
	}
	written, err := s.writePictures(normalized)
	if err != nil {
		return nil, nil, err
	}
	recipeFilenames := written[:len(pictures)]
	stepFilenames := make(map[int]string, len(stepIndices))
	for j, filename := range written[len(pictures):] {
		stepFilenames[stepIndices[j]] = filename
	}
	return recipeFilenames, stepFilenames, nil
}

func (s *Service) Create(ctx context.Context, authorID string, input models.CreateRecipe, pictures []PictureUpload, stepPictures map[int]PictureUpload) (models.Recipe, error) {
	locale, err := normalizeLocale(input.SourceLocale)
	if err != nil {
		return models.Recipe{}, err
	}
	input.SourceLocale = locale
	// Nothing exists yet on create: a step's picture can only come from a
	// fresh upload correlated by index below, never a client-supplied value.
	for i := range input.Steps {
		input.Steps[i].Picture = ""
	}
	recipeDB := models.RecipeDB{
		Title: input.Title, Description: input.Description, Quantity: input.Quantity, Kind: input.Kind,
		PreparationTime: input.PreparationTime, CookingTime: input.CookingTime, RestingTime: input.RestingTime,
		Ingredients: input.Ingredients, Steps: input.Steps, SourceLocale: input.SourceLocale,
	}
	if input.VariationOf != nil {
		target, err := s.db.GetRecipeById(input.VariationOf.Hex())
		if err != nil {
			if errors.Is(err, mongorepo.NotFoundError) {
				return models.Recipe{}, ErrNotFound
			}
			return models.Recipe{}, err
		}
		root := target.Id
		// Always flatten to the target's own root - a variation never chains
		// off another variation.
		if target.VariationOf != nil {
			root = target.VariationOf
		}
		recipeDB.VariationOf = root
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
	stepIndices, stepUploads, err := s.stepPictureUploads(stepPictures, len(recipeDB.Steps))
	if err != nil {
		return models.Recipe{}, err
	}
	recipeFilenames, stepFilenames, err := s.normalizeAndWriteAllPictures(pictures, stepUploads, stepIndices)
	if err != nil {
		return models.Recipe{}, err
	}
	recipeDB.Pictures = append(recipeDB.Pictures, recipeFilenames...)
	for index, filename := range stepFilenames {
		recipeDB.Steps[index].Picture = filename
	}
	recipeDB.SourceHash = sourceHash(recipeDB)
	create := utils.DupStruct[models.CreateRecipe](&recipeDB)
	created, err := s.db.CreateRecipe(authorID, &create)
	if err != nil {
		s.removePictures(append(recipeFilenames, mapValues(stepFilenames)...))
		return models.Recipe{}, err
	}
	s.scheduleTranslations(created)
	created.Locale = created.SourceLocale
	return created, nil
}

// mapValues returns m's values in unspecified order.
func mapValues[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// mergePatch applies patch onto recipe. freshStepPictures holds the step
// indices that have a new picture upload pending in this request: their
// patch-supplied Picture value is ignored (it will be overwritten with the
// upload's filename once normalized) rather than validated as a keep.
func mergePatch(recipe models.Recipe, patch models.UpdateRecipeRequest, freshStepPictures map[int]bool) (models.RecipeDB, error) {
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
		available := make(map[string]bool, len(recipe.Steps))
		for _, step := range recipe.Steps {
			if step.Picture != "" {
				available[step.Picture] = true
			}
		}
		for i := range merged.Steps {
			if freshStepPictures[i] {
				merged.Steps[i].Picture = ""
				continue
			}
			if merged.Steps[i].Picture != "" && !available[merged.Steps[i].Picture] {
				return models.RecipeDB{}, fmt.Errorf("%w: step %d references an unknown picture", ErrInvalid, i)
			}
		}
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

func (s *Service) Update(ctx context.Context, recipe models.Recipe, patch models.UpdateRecipeRequest, pictures []PictureUpload, stepPictures map[int]PictureUpload) (models.Recipe, error) {
	freshStepPictures := make(map[int]bool, len(stepPictures))
	for i := range stepPictures {
		freshStepPictures[i] = true
	}
	merged, err := mergePatch(recipe, patch, freshStepPictures)
	if err != nil {
		return models.Recipe{}, err
	}
	if err := validateRecipe(&merged); err != nil {
		return models.Recipe{}, err
	}
	stepIndices, stepUploads, err := s.stepPictureUploads(stepPictures, len(merged.Steps))
	if err != nil {
		return models.Recipe{}, err
	}
	recipeFilenames, stepFilenames, err := s.normalizeAndWriteAllPictures(pictures, stepUploads, stepIndices)
	if err != nil {
		return models.Recipe{}, err
	}
	merged.Pictures = append(merged.Pictures, recipeFilenames...)
	for index, filename := range stepFilenames {
		merged.Steps[index].Picture = filename
	}
	merged.SourceHash = sourceHash(merged)
	updated, err := s.db.ReplaceRecipeById(ctx, recipe.Id.Hex(), merged)
	if err != nil {
		s.removePictures(append(recipeFilenames, mapValues(stepFilenames)...))
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
	keptSteps := make(map[string]bool, len(updated.Steps))
	for _, step := range updated.Steps {
		if step.Picture != "" {
			keptSteps[step.Picture] = true
		}
	}
	for _, step := range recipe.Steps {
		if step.Picture != "" && !keptSteps[step.Picture] {
			_ = images_manager.Remove(s.imageDir, step.Picture)
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
	localized := s.localize(recipe, locale)
	// GetForUser is an internal author-scoped fetch (MCP), not a
	// social/discovery view, so it deliberately skips favorite decoration
	// (like Get does not) - but variation_count is still cheap and useful
	// for an MCP client managing its own recipes, so it's decorated here.
	s.decorateVariationCount(ctx, &localized)
	return localized, nil
}

func (s *Service) Get(ctx context.Context, recipeID, locale, userID string) (models.Recipe, error) {
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
	localized := s.localize(recipe, locale)
	// A detail page always represents one family, whichever member is being
	// viewed, so favorites are always decorated family-wide here (unlike
	// List, which has an "own recipes" per-recipe mode - see List).
	s.decorateFamilyFavorite(ctx, &localized, userID)
	s.decorateVariationCount(ctx, &localized)
	return localized, nil
}

// AddFavorite marks recipeID as favorited by userID; a no-op if it already is.
func (s *Service) AddFavorite(ctx context.Context, userID, recipeID string) error {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return ErrNotFound
	}
	if _, err := s.db.GetRecipeById(recipeID); err != nil {
		if errors.Is(err, mongorepo.NotFoundError) {
			return ErrNotFound
		}
		return err
	}
	return s.db.AddFavorite(ctx, userID, recipeID)
}

// RemoveFavorite un-favorites recipeID for userID; a no-op if it wasn't
// favorited (or no longer exists).
func (s *Service) RemoveFavorite(ctx context.Context, userID, recipeID string) error {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return ErrNotFound
	}
	return s.db.RemoveFavorite(ctx, userID, recipeID)
}

// decorateFavorite stamps Favorite/FavoriteCount onto recipe. userID may be
// empty (anonymous request); lookup failures are logged and leave the
// zero-value (not favorited, count 0) rather than failing the request.
func (s *Service) decorateFavorite(ctx context.Context, recipe *models.Recipe, userID string) {
	if recipe.Id == nil {
		return
	}
	info, err := s.db.GetFavoriteInfo(ctx, []string{recipe.Id.Hex()}, userID)
	if err != nil {
		utils.LogError("could not load favorite info", err)
		return
	}
	if fav, ok := info[recipe.Id.Hex()]; ok {
		recipe.Favorite = fav.Favorited
		recipe.FavoriteCount = fav.Count
	}
}

// decorateFavoritePreviews is decorateFavorite for a page of previews, fetched
// in one batched lookup.
func (s *Service) decorateFavoritePreviews(ctx context.Context, previews []models.RecipePreview, userID string) {
	ids := make([]string, 0, len(previews))
	for _, preview := range previews {
		if preview.Id != nil {
			ids = append(ids, preview.Id.Hex())
		}
	}
	if len(ids) == 0 {
		return
	}
	info, err := s.db.GetFavoriteInfo(ctx, ids, userID)
	if err != nil {
		utils.LogError("could not load favorite info", err)
		return
	}
	for i := range previews {
		if previews[i].Id == nil {
			continue
		}
		if fav, ok := info[previews[i].Id.Hex()]; ok {
			previews[i].Favorite = fav.Favorited
			previews[i].FavoriteCount = fav.Count
		}
	}
}

// familyRootHex returns id's own hex when it's a root, or its VariationOf's
// hex when it's a variation - the family root every family-wide decoration
// (favorites, variation count) is keyed by.
func familyRootHex(id, variationOf *primitive.ObjectID) string {
	if variationOf != nil {
		return variationOf.Hex()
	}
	if id != nil {
		return id.Hex()
	}
	return ""
}

// decorateFamilyFavorite is decorateFavorite, but counting distinct people
// who favorited any member of recipe's family (root or any variation), not
// just recipe itself - see decision #6. Used everywhere except "My Recipes"
// (List with OwnRecipes set), which wants each recipe's own individual count.
func (s *Service) decorateFamilyFavorite(ctx context.Context, recipe *models.Recipe, userID string) {
	rootHex := familyRootHex(recipe.Id, recipe.VariationOf)
	if rootHex == "" {
		return
	}
	info, err := s.db.GetFamilyFavoriteInfo(ctx, []string{rootHex}, userID)
	if err != nil {
		utils.LogError("could not load family favorite info", err)
		return
	}
	if fav, ok := info[rootHex]; ok {
		recipe.Favorite = fav.Favorited
		recipe.FavoriteCount = fav.Count
	}
}

// decorateFamilyFavoritePreviews is decorateFamilyFavorite for a page of
// previews, fetched in one batched lookup keyed by family root.
func (s *Service) decorateFamilyFavoritePreviews(ctx context.Context, previews []models.RecipePreview, userID string) {
	rootHexes := make([]string, 0, len(previews))
	seen := make(map[string]struct{}, len(previews))
	for _, preview := range previews {
		rootHex := familyRootHex(preview.Id, preview.VariationOf)
		if rootHex == "" {
			continue
		}
		if _, ok := seen[rootHex]; ok {
			continue
		}
		seen[rootHex] = struct{}{}
		rootHexes = append(rootHexes, rootHex)
	}
	if len(rootHexes) == 0 {
		return
	}
	info, err := s.db.GetFamilyFavoriteInfo(ctx, rootHexes, userID)
	if err != nil {
		utils.LogError("could not load family favorite info", err)
		return
	}
	for i := range previews {
		rootHex := familyRootHex(previews[i].Id, previews[i].VariationOf)
		if fav, ok := info[rootHex]; ok {
			previews[i].Favorite = fav.Favorited
			previews[i].FavoriteCount = fav.Count
		}
	}
}

// decorateVariationCount stamps VariationCount onto recipe; a no-op (count 0)
// when recipe is itself a variation, since VariationCount describes a root's
// family size, not "siblings of this variation".
func (s *Service) decorateVariationCount(ctx context.Context, recipe *models.Recipe) {
	if recipe.Id == nil || recipe.VariationOf != nil {
		return
	}
	counts, err := s.db.GetVariationCounts(ctx, []string{recipe.Id.Hex()})
	if err != nil {
		utils.LogError("could not load variation count", err)
		return
	}
	recipe.VariationCount = counts[recipe.Id.Hex()]
}

// decorateVariationCountsPreviews is decorateVariationCount for a page of
// previews, fetched in one batched lookup.
func (s *Service) decorateVariationCountsPreviews(ctx context.Context, previews []models.RecipePreview) {
	ids := make([]string, 0, len(previews))
	for _, preview := range previews {
		if preview.Id != nil && preview.VariationOf == nil {
			ids = append(ids, preview.Id.Hex())
		}
	}
	if len(ids) == 0 {
		return
	}
	counts, err := s.db.GetVariationCounts(ctx, ids)
	if err != nil {
		utils.LogError("could not load variation counts", err)
		return
	}
	for i := range previews {
		if previews[i].Id != nil && previews[i].VariationOf == nil {
			previews[i].VariationCount = counts[previews[i].Id.Hex()]
		}
	}
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
// what is actually being served. VariationOf always comes from canonical: it's a
// structural relationship, not translatable content, and recipe_translations rows
// never store it.
func pickTranslation(canonical, translation models.Recipe, requested string, found bool) models.Recipe {
	if found && translation.SourceHash != "" && translation.SourceHash == canonical.SourceHash {
		translation.Locale = requested
		translation.VariationOf = canonical.VariationOf
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

// List returns a page of recipes matching parameters. userID may be empty for
// an anonymous request; when non-empty, results are decorated with per-user
// favorite status, and if parameters.Favorite is set, every recipe userID has
// favorited (matching the filters) is returned first, unpaginated, ahead of a
// normal paginated page of the non-favorited remainder — see
// Store.GetRecipeDocumentsBoosted.
func (s *Service) List(ctx context.Context, parameters models.GetRecipesRequest, userID string) ([]models.RecipePreview, int64, error) {
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

	var documents []models.Recipe
	var count int64
	if parameters.Favorite && userID != "" {
		favorited, rest, total, err := s.db.GetRecipeDocumentsBoosted(ctx, userID, parameters)
		if err != nil {
			return nil, 0, err
		}
		documents = append(favorited, rest...)
		count = total
	} else {
		documents, count, err = s.db.GetRecipeDocuments(parameters)
		if err != nil {
			return nil, 0, err
		}
	}

	previews := s.localizePreviews(ctx, documents, parameters.Locale)
	if parameters.OwnRecipes {
		// "My Recipes": each entry's own individual favorite count, not the
		// family aggregate (decision #4).
		s.decorateFavoritePreviews(ctx, previews, userID)
	} else {
		s.decorateFamilyFavoritePreviews(ctx, previews, userID)
	}
	s.decorateVariationCountsPreviews(ctx, previews)
	return previews, count, nil
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
	previews := s.localizePreviews(ctx, documents, locale)
	s.decorateVariationCountsPreviews(ctx, previews)
	return previews, count, next, nil
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
			SourceLocale: recipe.SourceLocale, Locale: recipe.Locale, VariationOf: recipe.VariationOf,
		})
	}
	return previews
}

func (s *Service) Delete(ctx context.Context, recipeID string) (models.RecipeDB, error) {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return models.RecipeDB{}, ErrNotFound
	}
	favorites, err := s.db.GetFavoriteInfo(ctx, []string{recipeID}, "")
	if err != nil {
		return models.RecipeDB{}, err
	}
	if favorites[recipeID].Count > 0 {
		return models.RecipeDB{}, ErrHasFavorites
	}
	// If recipeID has variations, promote the oldest one to be the new root
	// and re-point the rest to it before deleting recipeID itself (decision
	// #2). No DB transactions are available, so this relies on ordering, not
	// atomicity, to stay crash-safe: a crash here either leaves recipeID
	// fully intact (safe retry from scratch) or leaves promotion already
	// applied with recipeID merely not-yet-deleted (a harmless, retry-safe
	// duplicate root until the retry finishes).
	oldestID, err := s.db.GetOldestVariationID(ctx, recipeID)
	if err != nil {
		return models.RecipeDB{}, err
	}
	if oldestID != nil {
		if err := s.db.RepointVariations(ctx, recipeID, oldestID.Hex()); err != nil {
			return models.RecipeDB{}, err
		}
		if err := s.db.PromoteRecipeToRoot(ctx, oldestID.Hex()); err != nil {
			return models.RecipeDB{}, err
		}
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
	for _, step := range deleted.Steps {
		if step.Picture != "" {
			_ = images_manager.Remove(s.imageDir, step.Picture)
		}
	}
	_ = s.db.DeleteLocalizedRecipesByID(ctx, recipeID)
	_ = s.db.DeleteFavoritesByRecipeID(ctx, recipeID)
	return deleted, nil
}
