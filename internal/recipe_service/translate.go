package recipe_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/models"
	"recipes/internal/utils"
)

// targetLocalesFor drops any configured target that shares a base language with
// the recipe's source locale.
func targetLocalesFor(source string, targets []string) []string {
	filtered := make([]string, 0, len(targets))
	for _, locale := range targets {
		if !sameBaseLocale(source, locale) {
			filtered = append(filtered, locale)
		}
	}
	return filtered
}

// scheduleTranslations kicks a detached goroutine that translates the recipe
// into every configured target locale. Fire-and-forget: failures are logged,
// not retried beyond translateLocale's own attempts, and lost on shutdown.
func (s *Service) scheduleTranslations(canonical models.Recipe) {
	if !s.canTranslate() || canonical.Id == nil || canonical.SourceHash == "" {
		return
	}
	go func() {
		if err := s.translateAll(canonical, true); err != nil {
			utils.LogError("translate recipe on write", err, "recipe", canonical.Id.Hex())
		}
	}()
}

// translateAll translates canonical into each target locale, holding one
// concurrency slot for the whole batch. Locales are attempted independently;
// the first error is returned.
func (s *Service) translateAll(canonical models.Recipe, force bool) error {
	s.translateSlots <- struct{}{}
	defer func() { <-s.translateSlots }()

	var firstErr error
	for _, locale := range targetLocalesFor(canonical.SourceLocale, s.targetLocales) {
		if err := s.translateLocale(canonical, locale, force); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// translateLocale translates one locale and stores the row. When force is false
// it first skips locales that already have a row matching the current source
// hash. Each locale gets len(translateBackoffs) attempts.
func (s *Service) translateLocale(canonical models.Recipe, locale string, force bool) error {
	if !force {
		if existing, err := s.db.GetRecipeByIdLocale(canonical.Id.Hex(), locale); err == nil &&
			existing.SourceHash != "" && existing.SourceHash == canonical.SourceHash {
			return nil
		}
	}
	var lastErr error
	for _, backoff := range translateBackoffs {
		time.Sleep(backoff)
		translated, err := s.translator.TranslateRecipe(canonical, locale)
		if err != nil {
			lastErr = err
			continue
		}
		translated.SourceLocale = canonical.SourceLocale
		translated.Locale = locale
		translated.SourceHash = canonical.SourceHash
		if _, err := s.db.AddLocaleRecipe(translated, locale); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// Retranslate re-runs translation for one recipe against its current canonical
// text. Intended for admin corrections; returns once the work is scheduled.
func (s *Service) Retranslate(ctx context.Context, recipeID string) error {
	if _, err := primitive.ObjectIDFromHex(recipeID); err != nil {
		return ErrNotFound
	}
	recipe, err := s.db.GetRecipeById(recipeID)
	if err != nil {
		if errors.Is(err, mongorepo.NotFoundError) {
			return ErrNotFound
		}
		return err
	}
	s.scheduleTranslations(recipe)
	return nil
}

// BackfillTranslations translates every existing recipe into the configured
// target locales, once. It first repairs missing source_locale/source_hash on
// the canonical document, which the read path's hash gate needs. The completion
// marker is written only after a pass with no per-recipe failure, so transient
// errors are retried on the next start.
func (s *Service) BackfillTranslations(ctx context.Context) {
	if !s.canTranslate() {
		return
	}
	done, err := s.db.TranslationBackfillCompleted(ctx)
	if err != nil {
		utils.LogError("translation backfill: read marker", err)
		return
	}
	if done {
		return
	}

	clean := true
	for offset := 0; ; offset += backfillPageSize {
		if ctx.Err() != nil {
			return
		}
		documents, _, err := s.db.GetRecipeDocuments(models.GetRecipesRequest{Limit: backfillPageSize, Offset: offset})
		if err != nil {
			utils.LogError("translation backfill: list recipes", err)
			return
		}
		if len(documents) == 0 {
			break
		}
		for _, document := range documents {
			if err := s.backfillRecipe(ctx, document); err != nil {
				clean = false
				utils.LogError("translation backfill: recipe", err, "recipe", recipeIDHex(document))
			}
		}
		if len(documents) < backfillPageSize {
			break
		}
	}
	if clean {
		if err := s.db.MarkTranslationBackfillCompleted(ctx); err != nil {
			utils.LogError("translation backfill: write marker", err)
		}
	}
}

func (s *Service) backfillRecipe(ctx context.Context, canonical models.Recipe) error {
	if canonical.Id == nil {
		return nil
	}
	if canonical.SourceLocale == "" || canonical.SourceHash == "" {
		repaired, err := s.repairCanonicalMetadata(ctx, canonical)
		if err != nil {
			return err
		}
		canonical = repaired
	}
	return s.translateAll(canonical, false)
}

// repairCanonicalMetadata detects the source locale when missing and persists
// source_locale + source_hash onto the canonical document.
func (s *Service) repairCanonicalMetadata(ctx context.Context, canonical models.Recipe) (models.Recipe, error) {
	if canonical.SourceLocale == "" {
		detected, err := s.translator.GetRecipeLocale(canonical)
		if err != nil {
			return models.Recipe{}, fmt.Errorf("detect source locale: %w", err)
		}
		normalized, err := normalizeLocale(detected)
		if err != nil || normalized == "" {
			return models.Recipe{}, fmt.Errorf("detect source locale: unrecognized %q", detected)
		}
		canonical.SourceLocale = normalized
	}
	recipeDB := canonical.ToRecipeDB()
	recipeDB.Locale = ""
	recipeDB.SourceHash = sourceHash(recipeDB)
	stored, err := s.db.ReplaceRecipeById(ctx, canonical.Id.Hex(), recipeDB)
	if err != nil {
		return models.Recipe{}, fmt.Errorf("persist canonical metadata: %w", err)
	}
	return stored, nil
}

func recipeIDHex(recipe models.Recipe) string {
	if recipe.Id == nil {
		return ""
	}
	return recipe.Id.Hex()
}
