package translator

import (
	"cloud.google.com/go/translate"
	"golang.org/x/net/context"
	"golang.org/x/text/language"
	"google.golang.org/api/option"
	"recipes/internal/models"
	"strings"
)

type Translator struct {
	client *translate.Client
}

type TranslatorConfig struct {
	ApiKey string
}

func New(cfg *TranslatorConfig) (*Translator, error) {
	translator := &Translator{}

	client, err := translate.NewClient(context.Background(), option.WithAPIKey(cfg.ApiKey))
	if err != nil {
		return nil, err
	}

	translator.client = client
	return translator, nil
}

func (t *Translator) TranslateRecipe(recipe models.Recipe, to string) (models.Recipe, error) {
	var ingredients []string
	var ingredientsUnit []string
	var stepTitles []string
	var stepDesc []string

	for _, ingredient := range recipe.Ingredients {
		ingredients = append(ingredients, ingredient.Name)
		ingredientsUnit = append(ingredientsUnit, ingredient.Unit)
	}
	for _, step := range recipe.Steps {
		stepTitles = append(stepTitles, step.Title)
		stepDesc = append(stepDesc, step.Description)
	}

	input := []string{
		recipe.Title,
		recipe.Description,
	}
	input = append(input, ingredients...)
	input = append(input, stepTitles...)
	input = append(input, stepDesc...)
	input = append(input, ingredientsUnit...)
	baseTo := language.MustParse(to)
	translations, err := t.client.Translate(context.TODO(), input, baseTo, &translate.Options{
		Format: translate.Text,
		Model:  "nmt",
	})
	if err != nil {
		return models.Recipe{}, err
	}

	var ing []models.Ingredient
	ingName := translations[2 : 2+len(ingredients)]
	ingUnit := translations[2+len(ingredients)+len(stepTitles)+len(stepDesc):]
	for i, ingredient := range recipe.Ingredients {
		ing = append(ing, models.Ingredient{
			Name:     ingName[i].Text,
			Quantity: ingredient.Quantity,
			Unit:     ingUnit[i].Text,
		})
	}
	var st []models.Step
	stTitle := translations[2+len(ingredients) : 2+len(ingredients)+len(stepTitles)]
	stDesc := translations[2+len(ingredients)+len(stepTitles) : 2+len(ingredients)+len(stepTitles)+len(stepDesc)]
	for i := range recipe.Steps {
		st = append(st, models.Step{
			Title:       stTitle[i].Text,
			Description: stDesc[i].Text,
		})
	}
	return models.Recipe{
		Author:          recipe.Author,
		Id:              recipe.Id,
		Title:           translations[0].Text,
		Description:     translations[1].Text,
		Quantity:        recipe.Quantity,
		Kind:            recipe.Kind,
		PreparationTime: recipe.PreparationTime,
		CookingTime:     recipe.CookingTime,
		RestingTime:     recipe.RestingTime,
		Ingredients:     ing,
		Steps:           st,
		Pictures:        recipe.Pictures,
	}, nil
}

func (t *Translator) GetRecipeLocale(recipe models.Recipe) (string, error) {
	var ingredients []string

	for _, ingredient := range recipe.Ingredients {
		ingredients = append(ingredients, ingredient.Name)
	}
	detected, err := t.client.DetectLanguage(context.TODO(), []string{strings.Join(ingredients, " ")})
	if err != nil {
		return "", err
	}

	highestIndex := float64(0)
	for i, verdict := range detected[0] {
		if verdict.Confidence > highestIndex {
			highestIndex = float64(i)
		}
	}
	return detected[0][int(highestIndex)].Language.String(), err
}
