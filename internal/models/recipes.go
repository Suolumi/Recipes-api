package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"recipes/internal/utils"
)

type GetRecipesRequest struct {
	Limit           int             `query:"limit,omitempty"`
	Offset          int             `query:"offset,omitempty"`
	Author          string          `query:"author,omitempty"`
	Title           string          `query:"title,omitempty"`
	PreparationTime *utils.Duration `query:"preparationTime,omitempty"`
	TotalTime       *utils.Duration `query:"totalTime"`
	Heating         []HeatingStyle  `query:"heating,omitempty"`
	Ingredients     []string        `query:"ingredients,omitempty"`
	Kind            RecipeKind      `query:"kind,omitempty"`
}

type GetRecipesResponse struct {
	Length int64           `json:"length"`
	Items  []RecipePreview `json:"items"`
}

type UpdateRecipeRequest struct {
	Title           string          `bson:"title,omitempty" json:"title,omitempty"`
	Description     string          `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        float64         `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind      `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime *utils.Duration `bson:"preparationTime,omitempty" json:"preparationTime,omitempty"`
	CookingTime     *utils.Duration `bson:"cookingTime,omitempty" json:"cookingTime,omitempty"`
	RestingTime     *utils.Duration `bson:"restingTime,omitempty" json:"restingTime,omitempty"`
	Heating         []HeatingStyle  `bson:"heating,omitempty" json:"heating,omitempty"`
	Ingredients     []Ingredient    `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step          `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string        `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type RecipeKind string

var RecipeKinds = []RecipeKind{
	Snack,
	Starter,
	Dish,
	SideDish,
	Sauce,
	Dessert,
	Drink,
	OtherRecipe,
}

const Snack = RecipeKind("snack")
const Starter = RecipeKind("starter")
const Dish = RecipeKind("dish")
const SideDish = RecipeKind("side-dish")
const Sauce = RecipeKind("sauce")
const Dessert = RecipeKind("dessert")
const Drink = RecipeKind("drink")
const OtherRecipe = RecipeKind("other")

type HeatingStyle string

var HeatingStyles = []HeatingStyle{
	Oven,
	Microwave,
	HotPlate,
	Barbecue,
	NoHeating,
	OtherHeating,
}

const Oven = HeatingStyle("oven")
const Microwave = HeatingStyle("microwave")
const HotPlate = HeatingStyle("hot-plate")
const Barbecue = HeatingStyle("barbecue")
const NoHeating = HeatingStyle("no-heating")
const OtherHeating = HeatingStyle("other")

type CreateRecipe struct {
	Title           string          `bson:"title,omitempty" json:"title,omitempty"`
	Description     string          `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        float64         `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind      `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime *utils.Duration `bson:"preparationTime,omitempty" json:"preparationTime,omitempty"`
	CookingTime     *utils.Duration `bson:"cookingTime,omitempty" json:"cookingTime,omitempty"`
	RestingTime     *utils.Duration `bson:"restingTime,omitempty" json:"restingTime,omitempty"`
	Heating         []HeatingStyle  `bson:"heating,omitempty" json:"heating,omitempty"`
	Ingredients     []Ingredient    `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step          `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string        `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type RecipeDB struct {
	Author          *primitive.ObjectID `bson:"author,omitempty" json:"author,omitempty"`
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	Description     string              `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        float64             `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime *utils.Duration     `bson:"preparationTime,omitempty" json:"preparationTime,omitempty"`
	CookingTime     *utils.Duration     `bson:"cookingTime,omitempty" json:"cookingTime,omitempty"`
	RestingTime     *utils.Duration     `bson:"restingTime,omitempty" json:"restingTime,omitempty"`
	Heating         []HeatingStyle      `bson:"heating,omitempty" json:"heating,omitempty"`
	Ingredients     []Ingredient        `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step              `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

// Recipe has bson fields to unfold the author when getting the document
type Recipe struct {
	Author          *UserView           `bson:"author,omitempty" json:"author,omitempty"`
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	Description     string              `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        float64             `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime *utils.Duration     `bson:"preparationTime,omitempty" json:"preparationTime,omitempty"`
	CookingTime     *utils.Duration     `bson:"cookingTime,omitempty" json:"cookingTime,omitempty"`
	RestingTime     *utils.Duration     `bson:"restingTime,omitempty" json:"restingTime,omitempty"`
	Heating         []HeatingStyle      `bson:"heating,omitempty" json:"heating,omitempty"`
	Ingredients     []Ingredient        `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step              `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type RecipePreview struct {
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	Author          *UserView           `bson:"author,omitempty" json:"author,omitempty"`
	PreparationTime *utils.Duration     `bson:"preparationTime,omitempty" json:"preparationTime,omitempty"`
	CookingTime     *utils.Duration     `bson:"cookingTime,omitempty" json:"cookingTime,omitempty"`
	RestingTime     *utils.Duration     `bson:"restingTime,omitempty" json:"restingTime,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	Quantity        float64             `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type Ingredient struct {
	Name     string  `bson:"name,omitempty" json:"name,omitempty"`
	Quantity float64 `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Unit     string  `bson:"unit,omitempty" json:"unit,omitempty"`
}

type Step struct {
	Title       string `bson:"title,omitempty" json:"title,omitempty"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`
}
