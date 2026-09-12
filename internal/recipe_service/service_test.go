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
	getRecipeByIdLocaleFn func(id, locale string) (models.Recipe, error)
	addLocaleRecipeFn     func(recipe models.Recipe, locale string) (models.Recipe, error)
	getRecipeByIdFn       func(id string) (models.Recipe, error)
	getRecipeByIdLocale   int
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
func (f *fakeStore) GetRecipeDocuments(models.GetRecipesRequest) ([]models.Recipe, int64, error) {
	return nil, 0, nil
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
func (f *fakeStore) DeleteRecipeById(string) (models.RecipeDB, error)           { return models.RecipeDB{}, nil }
func (f *fakeStore) DeleteLocalizedRecipesByID(context.Context, string) error   { return nil }
func (f *fakeStore) TranslationBackfillCompleted(context.Context) (bool, error) { return false, nil }
func (f *fakeStore) MarkTranslationBackfillCompleted(context.Context) error     { return nil }

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
