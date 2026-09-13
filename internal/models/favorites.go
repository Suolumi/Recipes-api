package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// FavoriteDB is one recipe's favorites: one document per recipe, listing every
// user who has favorited it.
type FavoriteDB struct {
	Id     *primitive.ObjectID  `bson:"_id,omitempty"`
	Recipe *primitive.ObjectID  `bson:"recipe"`
	Users  []primitive.ObjectID `bson:"users"`
}

// FavoriteInfo is the per-recipe favorite decoration stamped onto a response:
// the public total, and (when looked up for a specific user) whether they've
// favorited it.
type FavoriteInfo struct {
	Count     int64
	Favorited bool
}
