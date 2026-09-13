package recipe_service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

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
}

func (f *fakeStore) CreateRecipe(string, *models.CreateRecipe) (models.Recipe, error) {
	return models.Recipe{}, nil
}
func (f *fakeStore) ReplaceRecipeById(context.Context, string, models.RecipeDB) (models.Recipe, error) {
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
func (f *fakeStore) DeleteRecipeById(string) (models.RecipeDB, error)         { return models.RecipeDB{}, nil }
func (f *fakeStore) DeleteLocalizedRecipesByID(context.Context, string) error { return nil }

func (f *fakeStore) AddFavorite(context.Context, string, string) error       { return nil }
func (f *fakeStore) RemoveFavorite(context.Context, string, string) error    { return nil }
func (f *fakeStore) DeleteFavoritesByRecipeID(context.Context, string) error { return nil }
func (f *fakeStore) GetFavoriteInfo(_ context.Context, ids []string, userID string) (map[string]models.FavoriteInfo, error) {
	if f.getFavoriteInfoFn != nil {
		return f.getFavoriteInfoFn(ids, userID)
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
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
			if len(ids) != 1 || ids[0] != canonical.Id.Hex() || userID != "user-1" {
				t.Fatalf("GetFavoriteInfo called with ids=%v userID=%q", ids, userID)
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
		getRecipeByIdFn:   func(string) (models.Recipe, error) { return canonical, nil },
		getFavoriteInfoFn: func([]string, string) (map[string]models.FavoriteInfo, error) { return nil, errors.New("boom") },
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
		getFavoriteInfoFn: func(ids []string, userID string) (map[string]models.FavoriteInfo, error) {
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
