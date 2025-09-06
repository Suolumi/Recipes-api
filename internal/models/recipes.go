package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GetRecipesRequest struct {
	Limit           int        `query:"limit,omitempty"`
	Offset          int        `query:"offset,omitempty"`
	Author          string     `query:"author,omitempty"`
	Title           string     `query:"title,omitempty"`
	PreparationTime int        `query:"preparation_time,omitempty"`
	TotalTime       int        `query:"total_time"`
	Ingredients     []string   `query:"ingredients,omitempty"`
	Kind            RecipeKind `query:"kind,omitempty"`
}

type GetRecipesResponse struct {
	Length int64           `json:"length"`
	Items  []RecipePreview `json:"items"`
}

type UpdateRecipeRequest struct {
	Title           string       `bson:"title,omitempty" json:"title,omitempty"`
	Description     string       `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        int          `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind   `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime int          `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     int          `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     int          `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
	Ingredients     []Ingredient `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step       `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string     `bson:"pictures,omitempty" json:"pictures,omitempty"`
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
}

const Snack = RecipeKind("snack")
const Starter = RecipeKind("starter")
const Dish = RecipeKind("dish")
const SideDish = RecipeKind("side-dish")
const Sauce = RecipeKind("sauce")
const Dessert = RecipeKind("dessert")
const Drink = RecipeKind("drink")

type CreateRecipe struct {
	Title           string       `bson:"title,omitempty" json:"title,omitempty"`
	Description     string       `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        int          `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind   `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime int          `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     int          `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     int          `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
	Ingredients     []Ingredient `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step       `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string     `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type RecipeDB struct {
	Author          *primitive.ObjectID `bson:"author,omitempty" json:"author,omitempty"`
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	Description     string              `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
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
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
	Ingredients     []Ingredient        `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           []Step              `bson:"steps,omitempty" json:"steps,omitempty"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type RecipePreview struct {
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	Description     string              `bson:"description,omitempty" json:"description,omitempty"`
	Author          *UserView           `bson:"author,omitempty" json:"author,omitempty"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind,omitempty"`
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures,omitempty"`
}

type Ingredient struct {
	Name     string `bson:"name,omitempty" json:"name"`
	Quantity int    `bson:"quantity,omitempty" json:"quantity"`
	Unit     string `bson:"unit,omitempty" json:"unit"`
}

type Step struct {
	Title       string `bson:"title,omitempty" json:"title"`
	Description string `bson:"description,omitempty" json:"description"`
}
