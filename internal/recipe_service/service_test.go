package recipe_service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/models"
)

// fakeStore satisfies Store; only the methods a test needs are wired, the rest
// return zero values.
type fakeStore struct {
	getRecipeByIdLocaleFn       func(id, locale string) (models.Recipe, error)
	addLocaleRecipeFn           func(recipe models.Recipe, locale string) (models.Recipe, error)
	getRecipeByIdFn             func(id string) (models.Recipe, error)
	getRecipeByIdLocale         int
	getRecipeDocumentsFn        func(models.GetRecipesRequest) ([]models.Recipe, int64, error)
	getRecipeDocumentsBoostedFn func(userID string, parameters models.GetRecipesRequest) ([]models.Recipe, []models.Recipe, int64, error)
	getRecipeDocumentsBoosted   int
	getFavoriteInfoFn           func(ids []string, userID string) (map[string]models.FavoriteInfo, error)
	getFamilyFavoriteInfoFn     func(rootIDs []string, userID string) (map[string]models.FavoriteInfo, error)
	deleteRecipeByIdFn          func(id string) (models.RecipeDB, error)
	deleteRecipeById            int
	createRecipeFn              func(authorID string, infos *models.CreateRecipe) (models.Recipe, error)
	replaceRecipeByIdFn         func(id string, recipe models.RecipeDB) (models.Recipe, error)
	getOldestVariationIDFn      func(rootID string) (*primitive.ObjectID, error)
	repointVariationsFn         func(oldRootID, newRootID string) error
	promoteRecipeToRootFn       func(id string) error
	getVariationCountsFn        func(rootIDs []string) (map[string]int64, error)
	repointRecipeReferencesFn   func(oldRootID, newRootID string) error
	hasIncomingReferencesFn     func(recipeID string) (bool, error)
	getRecipeTitlesFn           func(ids []string) (map[string]string, error)
	// callOrder records, in order, the names of RepointVariations/
	// PromoteRecipeToRoot/DeleteRecipeById calls - used to assert promotion
	// happens strictly before the delete.
	callOrder               []string
	repointVariations       int
	promoteRecipeToRoot     int
	repointRecipeReferences int
}

func (f *fakeStore) CreateRecipe(authorID string, infos *models.CreateRecipe) (models.Recipe, error) {
	if f.createRecipeFn != nil {
		return f.createRecipeFn(authorID, infos)
	}
	return models.Recipe{}, nil
}
func (f *fakeStore) ReplaceRecipeById(_ context.Context, id string, recipe models.RecipeDB) (models.Recipe, error) {
	if f.replaceRecipeByIdFn != nil {
		return f.replaceRecipeByIdFn(id, recipe)
	}
	return models.Recipe{}, nil
}
func (f *fakeStore) GetRecipeById(id string) (models.Recipe, error) {
	if f.getRecipeByIdFn != nil {
		return f.getRecipeByIdFn(id)
	}
	return models.Recipe{}, nil
}
func (f *fakeStore) GetRecipeByIdForAuthor(context.Context, string, string) (models.Recipe, error) {
	return models.Recipe{}, nil
}
func (f *fakeStore) GetRecipeByIdLocale(id string, locale string) (models.Recipe, error) {
	f.getRecipeByIdLocale++
	if f.getRecipeByIdLocaleFn != nil {
		return f.getRecipeByIdLocaleFn(id, locale)
	}
	return models.Recipe{}, errors.New("not found")
}
func (f *fakeStore) GetRecipeDocuments(parameters models.GetRecipesRequest) ([]models.Recipe, int64, error) {
	if f.getRecipeDocumentsFn != nil {
		return f.getRecipeDocumentsFn(parameters)
	}
	return nil, 0, nil
}
func (f *fakeStore) GetRecipeDocumentsBoosted(_ context.Context, userID string, parameters models.GetRecipesRequest) ([]models.Recipe, []models.Recipe, int64, error) {
	f.getRecipeDocumentsBoosted++
	if f.getRecipeDocumentsBoostedFn != nil {
		return f.getRecipeDocumentsBoostedFn(userID, parameters)
	}
	return nil, nil, 0, nil
}
func (f *fakeStore) GetRecipesByAuthor(context.Context, string, string, int) ([]models.Recipe, int64, error) {
	return nil, 0, nil
}
func (f *fakeStore) GetTranslationsByRecipeIDs(context.Context, []string, string) (map[string]models.Recipe, error) {
	return map[string]models.Recipe{}, nil
}
func (f *fakeStore) AddLocaleRecipe(recipe models.Recipe, locale string) (models.Recipe, error) {
	if f.addLocaleRecipeFn != nil {
		return f.addLocaleRecipeFn(recipe, locale)
	}
	return recipe, nil
}
func (f *fakeStore) DeleteRecipeById(id string) (models.RecipeDB, error) {
	f.deleteRecipeById++
	f.callOrder = append(f.callOrder, "DeleteRecipeById")
	if f.deleteRecipeByIdFn != nil {
		return f.deleteRecipeByIdFn(id)
	}
	return models.RecipeDB{}, nil
}
func (f *fakeStore) DeleteLocalizedRecipesByID(context.Context, string) error { return nil }
func (f *fakeStore) GetOldestVariationID(_ context.Context, rootID string) (*primitive.ObjectID, error) {
	if f.getOldestVariationIDFn != nil {
		return f.getOldestVariationIDFn(rootID)
	}
	return nil, nil
}
func (f *fakeStore) RepointVariations(_ context.Context, oldRootID, newRootID string) error {
	f.repointVariations++
	f.callOrder = append(f.callOrder, "RepointVariations")
	if f.repointVariationsFn != nil {
		return f.repointVariationsFn(oldRootID, newRootID)
	}
	return nil
}
func (f *fakeStore) PromoteRecipeToRoot(_ context.Context, id string) error {
	f.promoteRecipeToRoot++
	f.callOrder = append(f.callOrder, "PromoteRecipeToRoot")
	if f.promoteRecipeToRootFn != nil {
		return f.promoteRecipeToRootFn(id)
	}
	return nil
}
func (f *fakeStore) GetVariationCounts(_ context.Context, rootIDs []string) (map[string]int64, error) {
	if f.getVariationCountsFn != nil {
		return f.getVariationCountsFn(rootIDs)
	}
	return map[string]int64{}, nil
}
func (f *fakeStore) RepointRecipeReferences(_ context.Context, oldRootID, newRootID string) error {
	f.repointRecipeReferences++
	f.callOrder = append(f.callOrder, "RepointRecipeReferences")
	if f.repointRecipeReferencesFn != nil {
		return f.repointRecipeReferencesFn(oldRootID, newRootID)
	}
	return nil
}
func (f *fakeStore) HasIncomingReferences(_ context.Context, recipeID string) (bool, error) {
	if f.hasIncomingReferencesFn != nil {
		return f.hasIncomingReferencesFn(recipeID)
	}
	return false, nil
}

func (f *fakeStore) AddFavorite(context.Context, string, string) error       { return nil }
func (f *fakeStore) RemoveFavorite(context.Context, string, string) error    { return nil }
func (f *fakeStore) DeleteFavoritesByRecipeID(context.Context, string) error { return nil }
func (f *fakeStore) GetFavoriteInfo(_ context.Context, ids []string, userID string) (map[string]models.FavoriteInfo, error) {
	if f.getFavoriteInfoFn != nil {
		return f.getFavoriteInfoFn(ids, userID)
	}
	return map[string]models.FavoriteInfo{}, nil
}
func (f *fakeStore) GetFamilyFavoriteInfo(_ context.Context, rootIDs []string, userID string) (map[string]models.FavoriteInfo, error) {
	if f.getFamilyFavoriteInfoFn != nil {
		return f.getFamilyFavoriteInfoFn(rootIDs, userID)
	}
	return map[string]models.FavoriteInfo{}, nil
}

type fakeTranslator struct {
	translateFn func(recipe models.Recipe, to string) (models.Recipe, error)
	detectFn    func(recipe models.Recipe) (string, error)
}

func (f fakeTranslator) TranslateRecipe(recipe models.Recipe, to string) (models.Recipe, error) {
	return f.translateFn(recipe, to)
}
func (f fakeTranslator) GetRecipeLocale(recipe models.Recipe) (string, error) {
	return f.detectFn(recipe)
}

func canonicalRecipe() models.Recipe {
	id := primitive.NewObjectID()
	return models.Recipe{Id: &id, SourceLocale: "en", SourceHash: "hash-v1", Title: "Pancakes"}
}

func TestLocalizeServesMatchingTranslation(t *testing.T) {
	canonical := canonicalRecipe()
	store := &fakeStore{getRecipeByIdLocaleFn: func(id, locale string) (models.Recipe, error) {
		return models.Recipe{Id: canonical.Id, SourceHash: "hash-v1", Title: "Crêpes"}, nil
	}}
	s := &Service{db: store}

	got := s.localize(canonical, "fr")
	if got.Title != "Crêpes" || got.Locale != "fr" {
		t.Fatalf("got title=%q locale=%q, want Crêpes/fr", got.Title, got.Locale)
	}
}

func TestLocalizeFallsBackOnStaleOrMissing(t *testing.T) {
	canonical := canonicalRecipe()

	t.Run("hash mismatch", func(t *testing.T) {
		store := &fakeStore{getRecipeByIdLocaleFn: func(id, locale string) (models.Recipe, error) {
			return models.Recipe{Id: canonical.Id, SourceHash: "hash-old", Title: "Vieux"}, nil
		}}
		got := (&Service{db: store}).localize(canonical, "fr")
		if got.Title != "Pancakes" || got.Locale != "en" {
			t.Fatalf("got title=%q locale=%q, want Pancakes/en", got.Title, got.Locale)
		}
	})

	t.Run("no row", func(t *testing.T) {
		store := &fakeStore{} // GetRecipeByIdLocale returns an error
		got := (&Service{db: store}).localize(canonical, "fr")
		if got.Title != "Pancakes" || got.Locale != "en" {
			t.Fatalf("got title=%q locale=%q, want Pancakes/en", got.Title, got.Locale)
		}
	})
}

func TestLocalizeSkipsLookupWhenPointless(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canonical models.Recipe
		requested string
	}{
		{"same base locale", canonicalRecipe(), "en-US"},
		{"empty request", canonicalRecipe(), ""},
		{"no source hash", models.Recipe{Id: canonicalRecipe().Id, SourceLocale: "en"}, "fr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			got := (&Service{db: store}).localize(tc.canonical, tc.requested)
			if store.getRecipeByIdLocale != 0 {
				t.Fatalf("GetRecipeByIdLocale called %d times, want 0", store.getRecipeByIdLocale)
			}
			if got.Locale != tc.canonical.SourceLocale {
				t.Fatalf("Locale = %q, want %q", got.Locale, tc.canonical.SourceLocale)
			}
		})
	}
}

func TestTargetLocalesFor(t *testing.T) {
	if got := targetLocalesFor("fr", []string{"en", "fr", "fi"}); !reflect.DeepEqual(got, []string{"en", "fi"}) {
		t.Fatalf("targetLocalesFor(fr) = %v, want [en fi]", got)
	}
	if got := targetLocalesFor("fr-FR", []string{"en", "fr", "fi"}); !reflect.DeepEqual(got, []string{"en", "fi"}) {
		t.Fatalf("targetLocalesFor(fr-FR) = %v, want [en fi]", got)
	}
	if got := targetLocalesFor("", []string{"en", "fr", "fi"}); !reflect.DeepEqual(got, []string{"en", "fr", "fi"}) {
		t.Fatalf("targetLocalesFor(empty) = %v, want all", got)
	}
}

func TestTranslateLocaleRetriesThenGivesUp(t *testing.T) {
	restore := translateBackoffs
	translateBackoffs = []time.Duration{0, 0, 0}
	defer func() { translateBackoffs = restore }()

	canonical := canonicalRecipe()
	boom := errors.New("google is down")
	calls := 0
	s := &Service{
		db:             &fakeStore{},
		translator:     fakeTranslator{translateFn: func(models.Recipe, string) (models.Recipe, error) { calls++; return models.Recipe{}, boom }},
		translateSlots: make(chan struct{}, translateConcurrency),
	}

	err := s.translateLocale(canonical, "fr", true)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if calls != len(translateBackoffs) {
		t.Fatalf("translate attempts = %d, want %d", calls, len(translateBackoffs))
	}
}

func TestTranslateLocaleSucceedsAfterRetry(t *testing.T) {
	restore := translateBackoffs
	translateBackoffs = []time.Duration{0, 0, 0}
	defer func() { translateBackoffs = restore }()

	canonical := canonicalRecipe()
	calls := 0
	var stored models.Recipe
	s := &Service{
		db: &fakeStore{addLocaleRecipeFn: func(recipe models.Recipe, locale string) (models.Recipe, error) {
			stored = recipe
			return recipe, nil
		}},
		translator: fakeTranslator{translateFn: func(r models.Recipe, to string) (models.Recipe, error) {
			calls++
			if calls < 2 {
				return models.Recipe{}, errors.New("transient")
			}
			return models.Recipe{Title: "Crêpes"}, nil
		}},
		translateSlots: make(chan struct{}, translateConcurrency),
	}

	if err := s.translateLocale(canonical, "fr", true); err != nil {
		t.Fatalf("translateLocale: %v", err)
	}
	if calls != 2 {
		t.Fatalf("translate attempts = %d, want 2", calls)
	}
	if stored.SourceHash != canonical.SourceHash || stored.SourceLocale != "en" || stored.Locale != "fr" {
		t.Fatalf("stored row = %+v, want hash/source/locale hash-v1/en/fr", stored)
	}
}

func TestTranslateLocaleSkipsFreshRowWhenNotForced(t *testing.T) {
	restore := translateBackoffs
	translateBackoffs = []time.Duration{0, 0, 0}
	defer func() { translateBackoffs = restore }()

	canonical := canonicalRecipe()
	store := &fakeStore{getRecipeByIdLocaleFn: func(id, locale string) (models.Recipe, error) {
		return models.Recipe{SourceHash: canonical.SourceHash}, nil
	}}
	translated := false
	s := &Service{
		db:             store,
		translator:     fakeTranslator{translateFn: func(models.Recipe, string) (models.Recipe, error) { translated = true; return models.Recipe{}, nil }},
		translateSlots: make(chan struct{}, translateConcurrency),
	}

	if err := s.translateLocale(canonical, "fr", false); err != nil {
		t.Fatalf("translateLocale: %v", err)
	}
	if translated {
		t.Fatal("translator was called for an already-fresh row")
	}
}

func TestGetDecoratesFavorite(t *testing.T) {
	canonical := canonicalRecipe()
	store := &fakeStore{
		getRecipeByIdFn: func(string) (models.Recipe, error) { return canonical, nil },
		getFamilyFavoriteInfoFn: func(rootIDs []string, userID string) (map[string]models.FavoriteInfo, error) {
			if len(rootIDs) != 1 || rootIDs[0] != canonical.Id.Hex() || userID != "user-1" {
				t.Fatalf("GetFamilyFavoriteInfo called with rootIDs=%v userID=%q", rootIDs, userID)
			}
			return map[string]models.FavoriteInfo{canonical.Id.Hex(): {Count: 3, Favorited: true}}, nil
		},
	}
	s := &Service{db: store}

	got, err := s.Get(context.Background(), canonical.Id.Hex(), "", "user-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Favorite || got.FavoriteCount != 3 {
		t.Fatalf("got Favorite=%v FavoriteCount=%d, want true/3", got.Favorite, got.FavoriteCount)
	}
}

func TestGetLeavesFavoriteZeroValueOnLookupError(t *testing.T) {
	canonical := canonicalRecipe()
	store := &fakeStore{
		getRecipeByIdFn:         func(string) (models.Recipe, error) { return canonical, nil },
		getFamilyFavoriteInfoFn: func([]string, string) (map[string]models.FavoriteInfo, error) { return nil, errors.New("boom") },
	}
	s := &Service{db: store}

	got, err := s.Get(context.Background(), canonical.Id.Hex(), "", "user-1")
	if err != nil {
		t.Fatalf("Get: %v, want success despite favorite lookup failure", err)
	}
	if got.Favorite || got.FavoriteCount != 0 {
		t.Fatalf("got Favorite=%v FavoriteCount=%d, want false/0", got.Favorite, got.FavoriteCount)
	}
}

func TestListBoostedMergesFavoritedFirstAndDecorates(t *testing.T) {
	favoritedID := primitive.NewObjectID()
	restID := primitive.NewObjectID()
	store := &fakeStore{
		getRecipeDocumentsBoostedFn: func(userID string, parameters models.GetRecipesRequest) ([]models.Recipe, []models.Recipe, int64, error) {
			if userID != "user-1" {
				t.Fatalf("GetRecipeDocumentsBoosted userID = %q, want user-1", userID)
			}
			return []models.Recipe{{Id: &favoritedID, Title: "Favorited"}},
				[]models.Recipe{{Id: &restID, Title: "Rest"}}, 5, nil
		},
		getFamilyFavoriteInfoFn: func(rootIDs []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{
				favoritedID.Hex(): {Count: 2, Favorited: true},
				restID.Hex():      {Count: 0, Favorited: false},
			}, nil
		},
	}
	s := &Service{db: store}

	previews, total, err := s.List(context.Background(), models.GetRecipesRequest{Favorite: true}, "user-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(previews) != 2 || previews[0].Id.Hex() != favoritedID.Hex() || previews[1].Id.Hex() != restID.Hex() {
		t.Fatalf("previews = %+v, want [favorited, rest] in that order", previews)
	}
	if !previews[0].Favorite || previews[0].FavoriteCount != 2 {
		t.Fatalf("favorited preview = %+v, want Favorite=true FavoriteCount=2", previews[0])
	}
	if previews[1].Favorite || previews[1].FavoriteCount != 0 {
		t.Fatalf("rest preview = %+v, want Favorite=false FavoriteCount=0", previews[1])
	}
}

func TestListIgnoresFavoriteFlagWhenAnonymous(t *testing.T) {
	store := &fakeStore{
		getRecipeDocumentsFn: func(models.GetRecipesRequest) ([]models.Recipe, int64, error) {
			return nil, 0, nil
		},
	}
	s := &Service{db: store}

	if _, _, err := s.List(context.Background(), models.GetRecipesRequest{Favorite: true}, ""); err != nil {
		t.Fatalf("List: %v", err)
	}
	if store.getRecipeDocumentsBoosted != 0 {
		t.Fatalf("GetRecipeDocumentsBoosted called %d times for an anonymous request, want 0", store.getRecipeDocumentsBoosted)
	}
}

func TestDeleteRejectsRecipeWithFavorites(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{id: {Count: 1}}, nil
		},
	}
	s := &Service{db: store}

	_, err := s.Delete(context.Background(), id)
	if !errors.Is(err, ErrHasFavorites) {
		t.Fatalf("Delete err = %v, want ErrHasFavorites", err)
	}
	if store.deleteRecipeById != 0 {
		t.Fatalf("DeleteRecipeById called %d times, want 0", store.deleteRecipeById)
	}
}

func TestDeleteAllowsRecipeWithoutFavorites(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.deleteRecipeById != 1 {
		t.Fatalf("DeleteRecipeById called %d times, want 1", store.deleteRecipeById)
	}
}

func validRecipeDB(ingredients []models.Ingredient) models.RecipeDB {
	return models.RecipeDB{
		Title:       "Title",
		Description: "Description",
		Quantity:    1,
		Kind:        models.RecipeKinds[0],
		Ingredients: ingredients,
		Steps:       []models.Step{{Description: "Do it"}},
	}
}

func TestValidateRecipeTrimsIngredientLabel(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour", Label: "  For the dough  "}})

	if err := validateRecipe(&recipe); err != nil {
		t.Fatalf("validateRecipe: %v", err)
	}
	if got := recipe.Ingredients[0].Label; got != "For the dough" {
		t.Fatalf("Label = %q, want %q", got, "For the dough")
	}
}

func TestValidateRecipeCanonicalizesLabelCasing(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{
		{Name: "Flour", Label: "For the Dough"},
		{Name: "Egg", Label: "for the dough"},
		{Name: "Apple", Label: "For The DOUGH"},
	})

	if err := validateRecipe(&recipe); err != nil {
		t.Fatalf("validateRecipe: %v", err)
	}
	for i, ingredient := range recipe.Ingredients {
		if ingredient.Label != "For the Dough" {
			t.Fatalf("Ingredients[%d].Label = %q, want canonical %q", i, ingredient.Label, "For the Dough")
		}
	}
}

func TestValidateRecipeRejectsOverlongLabel(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour", Label: strings.Repeat("a", maxIngredientFieldLength+1)}})

	if err := validateRecipe(&recipe); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validateRecipe err = %v, want ErrInvalid", err)
	}
}

func TestValidateRecipeAllowsEmptyLabel(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour"}})

	if err := validateRecipe(&recipe); err != nil {
		t.Fatalf("validateRecipe: %v", err)
	}
	if recipe.Ingredients[0].Label != "" {
		t.Fatalf("Label = %q, want empty", recipe.Ingredients[0].Label)
	}
}

func TestValidateRecipeNormalizesEmptyCategoryToFood(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour"}})
	recipe.Category = ""

	if err := validateRecipe(&recipe); err != nil {
		t.Fatalf("validateRecipe: %v", err)
	}
	if recipe.Category != models.Food {
		t.Fatalf("Category = %q, want %q", recipe.Category, models.Food)
	}
}

func TestValidateRecipeRejectsUnknownCategory(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour"}})
	recipe.Category = models.RecipeCategory("bogus")

	if err := validateRecipe(&recipe); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validateRecipe err = %v, want ErrInvalid", err)
	}
}

func TestValidateRecipeRequiresKindForFoodCategory(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Flour"}})
	recipe.Category = models.Food
	recipe.Kind = models.RecipeKind("bogus")

	if err := validateRecipe(&recipe); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validateRecipe err = %v, want ErrInvalid", err)
	}
}

func TestValidateRecipeSkipsKindForDiyCategory(t *testing.T) {
	recipe := validRecipeDB([]models.Ingredient{{Name: "Baking soda"}})
	recipe.Category = models.Diy
	recipe.Kind = ""

	if err := validateRecipe(&recipe); err != nil {
		t.Fatalf("validateRecipe: %v", err)
	}
}

// pictureUpload builds a small valid PNG upload for tests that exercise
// normalization/writing.
func pictureUpload(t *testing.T) PictureUpload {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return PictureUpload{Filename: "step.png", MediaType: "image/png", Data: buf.Bytes()}
}

func TestStepPictureUploadsValidatesRange(t *testing.T) {
	s := &Service{}

	t.Run("empty map is a no-op", func(t *testing.T) {
		indices, uploads, err := s.stepPictureUploads(nil, 3)
		if err != nil || indices != nil || uploads != nil {
			t.Fatalf("got (%v, %v, %v), want (nil, nil, nil)", indices, uploads, err)
		}
	})

	t.Run("rejects out-of-range index", func(t *testing.T) {
		_, _, err := s.stepPictureUploads(map[int]PictureUpload{2: {}}, 2)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("orders by ascending index", func(t *testing.T) {
		upload0, upload2 := PictureUpload{Filename: "a"}, PictureUpload{Filename: "b"}
		indices, uploads, err := s.stepPictureUploads(map[int]PictureUpload{2: upload2, 0: upload0}, 3)
		if err != nil {
			t.Fatalf("stepPictureUploads: %v", err)
		}
		if !reflect.DeepEqual(indices, []int{0, 2}) {
			t.Fatalf("indices = %v, want [0 2]", indices)
		}
		if uploads[0].Filename != "a" || uploads[1].Filename != "b" {
			t.Fatalf("uploads = %+v, want [a b] in order", uploads)
		}
	})
}

func TestCreateIgnoresClientSuppliedStepPictureAndUsesUpload(t *testing.T) {
	dir := t.TempDir()
	var stored models.CreateRecipe
	store := &fakeStore{createRecipeFn: func(_ string, infos *models.CreateRecipe) (models.Recipe, error) {
		stored = *infos
		return models.Recipe{Id: ptrObjectID(), SourceHash: infos.SourceHash, Steps: infos.Steps}, nil
	}}
	s := &Service{db: store, imageDir: dir, maxPictureBytes: 10 << 20}

	input := models.CreateRecipe{
		Title: "Pancakes", Description: "d", Quantity: 1, Kind: models.RecipeKinds[0],
		Ingredients: []models.Ingredient{{Name: "Flour"}},
		Steps:       []models.Step{{Description: "Mix", Picture: "attacker-supplied.jpg"}},
	}
	created, err := s.Create(context.Background(), "author-1", input, nil, map[int]PictureUpload{0: pictureUpload(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.Steps[0].Picture == "attacker-supplied.jpg" || stored.Steps[0].Picture == "" {
		t.Fatalf("Steps[0].Picture = %q, want a freshly generated filename, not the client-supplied one", stored.Steps[0].Picture)
	}
	if _, err := os.Stat(filepath.Join(dir, stored.Steps[0].Picture)); err != nil {
		t.Fatalf("uploaded step picture was not written to disk: %v", err)
	}
	if created.Steps[0].Picture != stored.Steps[0].Picture {
		t.Fatalf("created.Steps[0].Picture = %q, want %q", created.Steps[0].Picture, stored.Steps[0].Picture)
	}
}

func TestCreateRejectsOutOfRangeStepPictureIndex(t *testing.T) {
	s := &Service{db: &fakeStore{}, imageDir: t.TempDir(), maxPictureBytes: 10 << 20}
	input := models.CreateRecipe{
		Title: "T", Description: "d", Quantity: 1, Kind: models.RecipeKinds[0],
		Ingredients: []models.Ingredient{{Name: "Flour"}},
		Steps:       []models.Step{{Description: "Mix"}},
	}
	_, err := s.Create(context.Background(), "author-1", input, nil, map[int]PictureUpload{5: pictureUpload(t)})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func ptrObjectID() *primitive.ObjectID {
	id := primitive.NewObjectID()
	return &id
}

func TestMergePatchRejectsUnknownStepPicture(t *testing.T) {
	recipe := canonicalRecipe()
	recipe.Author = &models.UserView{Id: ptrObjectID()}
	recipe.Steps = []models.Step{{Description: "Mix", Picture: "known.jpg"}}
	patch := models.UpdateRecipeRequest{Steps: &[]models.Step{{Description: "Mix", Picture: "unknown.jpg"}}}

	_, err := mergePatch(recipe, patch, nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMergePatchAllowsKeepingOrDroppingKnownStepPicture(t *testing.T) {
	recipe := canonicalRecipe()
	recipe.Author = &models.UserView{Id: ptrObjectID()}
	recipe.Steps = []models.Step{{Description: "Mix", Picture: "known.jpg"}}
	patch := models.UpdateRecipeRequest{Steps: &[]models.Step{
		{Description: "Mix", Picture: "known.jpg"},
		{Description: "Bake"},
	}}

	merged, err := mergePatch(recipe, patch, nil)
	if err != nil {
		t.Fatalf("mergePatch: %v", err)
	}
	if merged.Steps[0].Picture != "known.jpg" {
		t.Fatalf("Steps[0].Picture = %q, want kept known.jpg", merged.Steps[0].Picture)
	}
	if merged.Steps[1].Picture != "" {
		t.Fatalf("Steps[1].Picture = %q, want empty", merged.Steps[1].Picture)
	}
}

func TestMergePatchSkipsValidationForFreshUploadIndex(t *testing.T) {
	recipe := canonicalRecipe()
	recipe.Author = &models.UserView{Id: ptrObjectID()}
	recipe.Steps = []models.Step{{Description: "Mix"}}
	patch := models.UpdateRecipeRequest{Steps: &[]models.Step{{Description: "Mix", Picture: "whatever-will-be-overwritten.jpg"}}}

	merged, err := mergePatch(recipe, patch, map[int]bool{0: true})
	if err != nil {
		t.Fatalf("mergePatch: %v", err)
	}
	if merged.Steps[0].Picture != "" {
		t.Fatalf("Steps[0].Picture = %q, want cleared pending the fresh upload", merged.Steps[0].Picture)
	}
}

func TestUpdateRemovesDroppedStepPictureFromDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old-step.jpg"), []byte("data"), 0o600); err != nil {
		t.Fatalf("seed old picture: %v", err)
	}
	recipe := canonicalRecipe()
	recipe.Author = &models.UserView{Id: ptrObjectID()}
	recipe.Quantity = 1
	recipe.Kind = models.RecipeKinds[0]
	recipe.Ingredients = []models.Ingredient{{Name: "Flour"}}
	recipe.Steps = []models.Step{{Description: "Mix", Picture: "old-step.jpg"}}
	store := &fakeStore{replaceRecipeByIdFn: func(_ string, r models.RecipeDB) (models.Recipe, error) {
		return models.Recipe{Id: recipe.Id, SourceHash: r.SourceHash, Steps: r.Steps}, nil
	}}
	s := &Service{db: store, imageDir: dir, maxPictureBytes: 10 << 20}

	patch := models.UpdateRecipeRequest{Steps: &[]models.Step{{Description: "Mix"}}}
	if _, err := s.Update(context.Background(), recipe, patch, nil, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-step.jpg")); !os.IsNotExist(err) {
		t.Fatalf("old-step.jpg still exists after being dropped, err = %v", err)
	}
}

func TestUpdateWritesNewStepPictureAtGivenIndex(t *testing.T) {
	dir := t.TempDir()
	recipe := canonicalRecipe()
	recipe.Author = &models.UserView{Id: ptrObjectID()}
	recipe.Quantity = 1
	recipe.Kind = models.RecipeKinds[0]
	recipe.Ingredients = []models.Ingredient{{Name: "Flour"}}
	recipe.Steps = []models.Step{{Description: "Mix"}}
	var storedSteps []models.Step
	store := &fakeStore{replaceRecipeByIdFn: func(_ string, r models.RecipeDB) (models.Recipe, error) {
		storedSteps = r.Steps
		return models.Recipe{Id: recipe.Id, SourceHash: r.SourceHash, Steps: r.Steps}, nil
	}}
	s := &Service{db: store, imageDir: dir, maxPictureBytes: 10 << 20}

	patch := models.UpdateRecipeRequest{Steps: &[]models.Step{{Description: "Mix"}}}
	if _, err := s.Update(context.Background(), recipe, patch, nil, map[int]PictureUpload{0: pictureUpload(t)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if storedSteps[0].Picture == "" {
		t.Fatal("Steps[0].Picture is empty, want the freshly uploaded filename")
	}
	if _, err := os.Stat(filepath.Join(dir, storedSteps[0].Picture)); err != nil {
		t.Fatalf("new step picture was not written to disk: %v", err)
	}
}

func TestDeleteRemovesStepPicturesFromDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "step-pic.jpg"), []byte("data"), 0o600); err != nil {
		t.Fatalf("seed picture: %v", err)
	}
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
		deleteRecipeByIdFn: func(string) (models.RecipeDB, error) {
			return models.RecipeDB{Steps: []models.Step{{Description: "Mix", Picture: "step-pic.jpg"}}}, nil
		},
	}
	s := &Service{db: store, imageDir: dir}

	if _, err := s.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "step-pic.jpg")); !os.IsNotExist(err) {
		t.Fatalf("step-pic.jpg still exists after recipe deletion, err = %v", err)
	}
}

func validCreateRecipe() models.CreateRecipe {
	return models.CreateRecipe{
		Title: "Cinnamon Rolls", Description: "d", Quantity: 1, Kind: models.RecipeKinds[0],
		Ingredients: []models.Ingredient{{Name: "Flour"}},
		Steps:       []models.Step{{Description: "Mix"}},
	}
}

func TestCreateResolvesVariationRoot(t *testing.T) {
	targetID := ptrObjectID()
	target := models.Recipe{Id: targetID, Author: &models.UserView{Id: ptrObjectID()}}
	var stored models.CreateRecipe
	store := &fakeStore{
		getRecipeByIdFn: func(id string) (models.Recipe, error) {
			if id != targetID.Hex() {
				t.Fatalf("GetRecipeById called with %q, want %q", id, targetID.Hex())
			}
			return target, nil
		},
		createRecipeFn: func(_ string, infos *models.CreateRecipe) (models.Recipe, error) {
			stored = *infos
			return models.Recipe{Id: ptrObjectID(), SourceHash: infos.SourceHash}, nil
		},
	}
	s := &Service{db: store, imageDir: t.TempDir(), maxPictureBytes: 10 << 20}

	input := validCreateRecipe()
	input.VariationOf = targetID
	if _, err := s.Create(context.Background(), "author-1", input, nil, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.VariationOf == nil || stored.VariationOf.Hex() != targetID.Hex() {
		t.Fatalf("stored.VariationOf = %v, want %v", stored.VariationOf, targetID)
	}
}

func TestCreateFlattensVariationOfVariation(t *testing.T) {
	rootID := ptrObjectID()
	targetID := ptrObjectID()
	// target is itself a variation of rootID.
	target := models.Recipe{Id: targetID, VariationOf: rootID, Author: &models.UserView{Id: ptrObjectID()}}
	var stored models.CreateRecipe
	store := &fakeStore{
		getRecipeByIdFn: func(string) (models.Recipe, error) { return target, nil },
		createRecipeFn: func(_ string, infos *models.CreateRecipe) (models.Recipe, error) {
			stored = *infos
			return models.Recipe{Id: ptrObjectID(), SourceHash: infos.SourceHash}, nil
		},
	}
	s := &Service{db: store, imageDir: t.TempDir(), maxPictureBytes: 10 << 20}

	input := validCreateRecipe()
	input.VariationOf = targetID
	if _, err := s.Create(context.Background(), "author-1", input, nil, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.VariationOf == nil || stored.VariationOf.Hex() != rootID.Hex() {
		t.Fatalf("stored.VariationOf = %v, want flattened root %v, not the intermediate variation", stored.VariationOf, rootID)
	}
}

func TestCreateVariationOfMissingTargetReturnsNotFound(t *testing.T) {
	store := &fakeStore{
		getRecipeByIdFn: func(string) (models.Recipe, error) { return models.Recipe{}, mongorepo.NotFoundError },
	}
	s := &Service{db: store, imageDir: t.TempDir(), maxPictureBytes: 10 << 20}

	input := validCreateRecipe()
	input.VariationOf = ptrObjectID()
	_, err := s.Create(context.Background(), "author-1", input, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Create err = %v, want ErrNotFound", err)
	}
}

func TestSourceHashIgnoresVariationOf(t *testing.T) {
	base := validRecipeDB(nil)
	base.Ingredients = []models.Ingredient{{Name: "Flour"}}
	withVariation := base
	withVariation.VariationOf = ptrObjectID()

	if sourceHash(base) != sourceHash(withVariation) {
		t.Fatal("sourceHash differs based on VariationOf alone, want it to depend only on content")
	}
}

func TestDeleteWithNoVariationsSkipsPromotion(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.repointVariations != 0 || store.promoteRecipeToRoot != 0 {
		t.Fatalf("promotion calls = repoint:%d promote:%d, want 0/0", store.repointVariations, store.promoteRecipeToRoot)
	}
	if store.deleteRecipeById != 1 {
		t.Fatalf("DeleteRecipeById called %d times, want 1", store.deleteRecipeById)
	}
}

func TestDeletePromotesOldestVariationBeforeDeletingRoot(t *testing.T) {
	rootID := primitive.NewObjectID().Hex()
	oldestVariation := ptrObjectID()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
		getOldestVariationIDFn: func(gotRootID string) (*primitive.ObjectID, error) {
			if gotRootID != rootID {
				t.Fatalf("GetOldestVariationID called with %q, want %q", gotRootID, rootID)
			}
			return oldestVariation, nil
		},
		repointVariationsFn: func(oldRootID, newRootID string) error {
			if oldRootID != rootID || newRootID != oldestVariation.Hex() {
				t.Fatalf("RepointVariations(%q, %q), want (%q, %q)", oldRootID, newRootID, rootID, oldestVariation.Hex())
			}
			return nil
		},
		promoteRecipeToRootFn: func(id string) error {
			if id != oldestVariation.Hex() {
				t.Fatalf("PromoteRecipeToRoot(%q), want %q", id, oldestVariation.Hex())
			}
			return nil
		},
		repointRecipeReferencesFn: func(oldRootID, newRootID string) error {
			if oldRootID != rootID || newRootID != oldestVariation.Hex() {
				t.Fatalf("RepointRecipeReferences(%q, %q), want (%q, %q)", oldRootID, newRootID, rootID, oldestVariation.Hex())
			}
			return nil
		},
		hasIncomingReferencesFn: func(string) (bool, error) {
			t.Fatal("HasIncomingReferences should not be called when a variation is promoted in the root's place")
			return false, nil
		},
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), rootID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.repointVariations != 1 || store.promoteRecipeToRoot != 1 || store.repointRecipeReferences != 1 || store.deleteRecipeById != 1 {
		t.Fatalf("call counts = repoint:%d promote:%d repointRefs:%d delete:%d, want 1/1/1/1", store.repointVariations, store.promoteRecipeToRoot, store.repointRecipeReferences, store.deleteRecipeById)
	}
	want := []string{"RepointVariations", "PromoteRecipeToRoot", "RepointRecipeReferences", "DeleteRecipeById"}
	if len(store.callOrder) != len(want) {
		t.Fatalf("callOrder = %v, want %v", store.callOrder, want)
	}
	for i, name := range want {
		if store.callOrder[i] != name {
			t.Fatalf("callOrder = %v, want %v", store.callOrder, want)
		}
	}
}

func TestDeleteStillBlockedByFavoritesEvenWithVariations(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{id: {Count: 1}}, nil
		},
		getOldestVariationIDFn: func(string) (*primitive.ObjectID, error) {
			t.Fatal("GetOldestVariationID should not be called when the favorites guard already blocks the delete")
			return nil, nil
		},
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), id); !errors.Is(err, ErrHasFavorites) {
		t.Fatalf("Delete err = %v, want ErrHasFavorites", err)
	}
	if store.repointVariations != 0 || store.promoteRecipeToRoot != 0 {
		t.Fatalf("promotion calls = repoint:%d promote:%d, want 0/0", store.repointVariations, store.promoteRecipeToRoot)
	}
}

func TestDeleteBlockedWhenReferencedWithNoVariationToPromote(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
		hasIncomingReferencesFn: func(gotID string) (bool, error) {
			if gotID != id {
				t.Fatalf("HasIncomingReferences called with %q, want %q", gotID, id)
			}
			return true, nil
		},
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), id); !errors.Is(err, ErrRecipeReferenced) {
		t.Fatalf("Delete err = %v, want ErrRecipeReferenced", err)
	}
	if store.deleteRecipeById != 0 {
		t.Fatalf("DeleteRecipeById called %d times, want 0", store.deleteRecipeById)
	}
}

func TestDeleteAllowedWhenNotReferenced(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	store := &fakeStore{
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{}, nil
		},
		hasIncomingReferencesFn: func(string) (bool, error) { return false, nil },
	}
	s := &Service{db: store}

	if _, err := s.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.deleteRecipeById != 1 {
		t.Fatalf("DeleteRecipeById called %d times, want 1", store.deleteRecipeById)
	}
}

func TestGetDecoratesVariationCount(t *testing.T) {
	canonical := canonicalRecipe()
	store := &fakeStore{
		getRecipeByIdFn: func(string) (models.Recipe, error) { return canonical, nil },
		getVariationCountsFn: func(ids []string) (map[string]int64, error) {
			if len(ids) != 1 || ids[0] != canonical.Id.Hex() {
				t.Fatalf("GetVariationCounts called with %v, want [%s]", ids, canonical.Id.Hex())
			}
			return map[string]int64{canonical.Id.Hex(): 2}, nil
		},
	}
	s := &Service{db: store}

	got, err := s.Get(context.Background(), canonical.Id.Hex(), "", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.VariationCount != 2 {
		t.Fatalf("VariationCount = %d, want 2", got.VariationCount)
	}
}

func TestListUsesPerRecipeFavoritesForOwnRecipes(t *testing.T) {
	id := ptrObjectID()
	store := &fakeStore{
		getRecipeDocumentsFn: func(models.GetRecipesRequest) ([]models.Recipe, int64, error) {
			return []models.Recipe{{Id: id}}, 1, nil
		},
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			return map[string]models.FavoriteInfo{id.Hex(): {Count: 1, Favorited: true}}, nil
		},
		getFamilyFavoriteInfoFn: func([]string, string) (map[string]models.FavoriteInfo, error) {
			t.Fatal("GetFamilyFavoriteInfo should not be called in OwnRecipes mode")
			return nil, nil
		},
	}
	s := &Service{db: store}

	previews, _, err := s.List(context.Background(), models.GetRecipesRequest{OwnRecipes: true}, "user-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(previews) != 1 || !previews[0].Favorite || previews[0].FavoriteCount != 1 {
		t.Fatalf("previews = %+v, want one favorited preview with count 1", previews)
	}
}
