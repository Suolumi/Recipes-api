package models

import (
	"encoding/json"

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
	Locale          string     `query:"locale,omitempty"`
	SearchLocale    string     `query:"search_locale,omitempty"`
	Favorite        bool       `query:"favorite,omitempty"`
	// VariationOf, when set, lists only the variations of that recipe id
	// (never the root itself) instead of the default root-only listing.
	VariationOf string `query:"variation_of,omitempty"`
	// OwnRecipes switches List into "My Recipes" mode: every recipe matching
	// Author is returned flatly (roots and variations alike, no collapsing),
	// and favorite decoration is per-recipe instead of family-aggregate. Set
	// only by the Settings page's own-recipes fetch.
	OwnRecipes bool `query:"own_recipes,omitempty"`
}

type GetRecipesResponse struct {
	Length int64           `json:"length"`
	Items  []RecipePreview `json:"items"`
}

type UpdateRecipeRequest struct {
	Title           *string       `bson:"title,omitempty" json:"title,omitempty"`
	Description     *string       `bson:"description,omitempty" json:"description,omitempty"`
	Quantity        *int          `bson:"quantity,omitempty" json:"quantity,omitempty"`
	Kind            *RecipeKind   `bson:"kind,omitempty" json:"kind,omitempty"`
	PreparationTime *int          `bson:"preparation_time,omitempty" json:"preparation_time,omitempty"`
	CookingTime     *int          `bson:"cooking_time,omitempty" json:"cooking_time,omitempty"`
	RestingTime     *int          `bson:"resting_time,omitempty" json:"resting_time,omitempty"`
	Ingredients     *[]Ingredient `bson:"ingredients,omitempty" json:"ingredients,omitempty"`
	Steps           *[]Step       `bson:"steps,omitempty" json:"steps,omitempty"`
	Locale          *string       `bson:"source_locale,omitempty" json:"locale,omitempty"`
	KeepPictureIDs  *[]string     `bson:"-" json:"keep_picture_ids,omitempty"`
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
	Plate,
}

const Snack = RecipeKind("snack")
const Starter = RecipeKind("starter")
const Dish = RecipeKind("dish")
const SideDish = RecipeKind("side-dish")
const Sauce = RecipeKind("sauce")
const Dessert = RecipeKind("dessert")
const Drink = RecipeKind("drink")
const Plate = RecipeKind("plate")

type CreateRecipe struct {
	Title           string       `bson:"title,omitempty" json:"title"`
	Description     string       `bson:"description,omitempty" json:"description"`
	Quantity        int          `bson:"quantity,omitempty" json:"quantity"`
	Kind            RecipeKind   `bson:"kind,omitempty" json:"kind"`
	PreparationTime int          `bson:"preparation_time,omitempty" json:"preparation_time"`
	CookingTime     int          `bson:"cooking_time,omitempty" json:"cooking_time"`
	RestingTime     int          `bson:"resting_time,omitempty" json:"resting_time"`
	Ingredients     []Ingredient `bson:"ingredients,omitempty" json:"ingredients"`
	Steps           []Step       `bson:"steps,omitempty" json:"steps"`
	Pictures        []string     `bson:"pictures,omitempty" json:"-"`
	SourceLocale    string       `bson:"source_locale,omitempty" json:"locale,omitempty"`
	SourceHash      string       `bson:"source_hash,omitempty" json:"-"`
	// VariationOf, on the way in, is the id of the recipe being forked; the
	// service resolves it to that recipe's root (flattening a
	// variation-of-a-variation) before it's ever persisted. Must stay the
	// same *primitive.ObjectID type as RecipeDB.VariationOf: Create() passes
	// values through utils.DupStruct in both directions between these two
	// structs (service.go, mongo/recipes.go), which copies same-named fields
	// with a raw reflect.Value.Set that panics on a type mismatch. A plain
	// hex string unmarshals into this directly, same as any other id field.
	VariationOf *primitive.ObjectID `bson:"variation_of,omitempty" json:"variation_of,omitempty"`
}

type RecipeDB struct {
	Author          *primitive.ObjectID `bson:"author,omitempty" json:"author"`
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title           string              `bson:"title,omitempty" json:"title"`
	Description     string              `bson:"description,omitempty" json:"description"`
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time"`
	Ingredients     []Ingredient        `bson:"ingredients,omitempty" json:"ingredients"`
	Steps           []Step              `bson:"steps,omitempty" json:"steps"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures"`
	SourceLocale    string              `bson:"source_locale,omitempty" json:"source_locale,omitempty"`
	Locale          string              `bson:"locale,omitempty" json:"locale,omitempty"`
	SourceHash      string              `bson:"source_hash,omitempty" json:"-"`
	// VariationOf is nil for a root/original recipe, or the root recipe's id
	// for a variation - always flattened, never chained.
	VariationOf *primitive.ObjectID `bson:"variation_of,omitempty" json:"variation_of,omitempty"`
}

// Recipe has bson fields to unfold the author when getting the document
type Recipe struct {
	Author          *UserView           `bson:"author,omitempty" json:"author"`
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title           string              `bson:"title,omitempty" json:"title"`
	Description     string              `bson:"description,omitempty" json:"description"`
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time"`
	Ingredients     []Ingredient        `bson:"ingredients,omitempty" json:"ingredients"`
	Steps           []Step              `bson:"steps,omitempty" json:"steps"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures"`
	SourceLocale    string              `bson:"source_locale,omitempty" json:"source_locale,omitempty"`
	Locale          string              `bson:"locale,omitempty" json:"locale,omitempty"`
	SourceHash      string              `bson:"source_hash,omitempty" json:"-"`
	VariationOf     *primitive.ObjectID `bson:"variation_of,omitempty" json:"variation_of,omitempty"`
	// Favorite, FavoriteCount and VariationCount are stamped on after fetch
	// (see recipe_service.decorateFamilyFavorite/decorateVariationCount);
	// they never come from the recipe or translation document itself, hence
	// bson:"-".
	Favorite       bool  `bson:"-" json:"favorite"`
	FavoriteCount  int64 `bson:"-" json:"favorite_count"`
	VariationCount int64 `bson:"-" json:"variation_count"`
}

// MarshalJSON ensures a recipe with no pictures serializes `pictures` as `[]`
// rather than `null`: a nil slice is a valid, common state (see docs/mcp.md,
// "Zero pictures is valid"), but clients that assume an array (e.g. the
// website's `formData.pictures.length` check) crash on `null`.
func (r Recipe) MarshalJSON() ([]byte, error) {
	type alias Recipe
	a := alias(r)
	if a.Pictures == nil {
		a.Pictures = []string{}
	}
	return json.Marshal(a)
}

func (r *Recipe) ToRecipeDB() RecipeDB {
	return RecipeDB{
		Author:          r.Author.Id,
		Id:              r.Id,
		Title:           r.Title,
		Description:     r.Description,
		Quantity:        r.Quantity,
		Kind:            r.Kind,
		PreparationTime: r.PreparationTime,
		CookingTime:     r.CookingTime,
		RestingTime:     r.RestingTime,
		Ingredients:     r.Ingredients,
		Steps:           r.Steps,
		Pictures:        r.Pictures,
		SourceLocale:    r.SourceLocale,
		Locale:          r.Locale,
		SourceHash:      r.SourceHash,
		VariationOf:     r.VariationOf,
	}
}

type RecipePreview struct {
	Id              *primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title           string              `bson:"title,omitempty" json:"title"`
	Description     string              `bson:"description,omitempty" json:"description"`
	Author          *UserView           `bson:"author,omitempty" json:"author"`
	PreparationTime int                 `bson:"preparation_time,omitempty" json:"preparation_time"`
	CookingTime     int                 `bson:"cooking_time,omitempty" json:"cooking_time"`
	RestingTime     int                 `bson:"resting_time,omitempty" json:"resting_time"`
	Kind            RecipeKind          `bson:"kind,omitempty" json:"kind"`
	Quantity        int                 `bson:"quantity,omitempty" json:"quantity"`
	Pictures        []string            `bson:"pictures,omitempty" json:"pictures"`
	SourceLocale    string              `bson:"source_locale,omitempty" json:"source_locale,omitempty"`
	Locale          string              `bson:"locale,omitempty" json:"locale,omitempty"`
	VariationOf     *primitive.ObjectID `bson:"variation_of,omitempty" json:"variation_of,omitempty"`
	Favorite        bool                `bson:"-" json:"favorite"`
	FavoriteCount   int64               `bson:"-" json:"favorite_count"`
	VariationCount  int64               `bson:"-" json:"variation_count"`
}

// MarshalJSON ensures `pictures` serializes as `[]` rather than `null`; see
// Recipe.MarshalJSON for why.
func (r RecipePreview) MarshalJSON() ([]byte, error) {
	type alias RecipePreview
	a := alias(r)
	if a.Pictures == nil {
		a.Pictures = []string{}
	}
	return json.Marshal(a)
}

type Ingredient struct {
	Name     string  `bson:"name,omitempty" json:"name" jsonschema:"Ingredient name, e.g. 'Egg' or 'Thyme'"`
	Quantity float64 `bson:"quantity,omitempty" json:"quantity" jsonschema:"Numeric amount, e.g. 3 or 0.5"`
	Unit     string  `bson:"unit,omitempty" json:"unit" jsonschema:"Optional unit shown between quantity and name. Leave empty for a bare count, e.g. quantity 3 + name 'Egg' renders as '3 Egg'. Set it for a unit of measure or descriptor, e.g. quantity 3 + unit 'leaves' + name 'Thyme' renders as '3 leaves - Thyme'"`
	Label    string  `bson:"label,omitempty" json:"label" jsonschema:"Optional section heading grouping this ingredient with others that share the exact same label, e.g. 'For the dough' or 'For the filling'. Leave empty for ingredients that don't belong to a named section. Reuse the identical label text (same wording and case) on every ingredient meant to share a section - matching is normalized server-side but exact reuse is still the reliable way to keep a group together."`
}

type Step struct {
	Title       string `bson:"title,omitempty" json:"title"`
	Description string `bson:"description,omitempty" json:"description"`
	Picture     string `bson:"picture,omitempty" json:"picture,omitempty" jsonschema:"Filename of an existing picture already attached to one of this recipe's steps, or empty. New pictures cannot be uploaded through MCP; attach photos via the website."`
}
