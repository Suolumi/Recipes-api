package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// Requests / Responses models

type UpdateUser struct {
	Password string `bson:"password,omitempty" json:"password,omitempty"`
	Username string `bson:"username,omitempty" json:"username,omitempty"`
	Email    string `bson:"email,omitempty" json:"email,omitempty"`
	Picture  string `bson:"picture,omitempty" json:"picture,omitempty"`
}

type UpdatePictureResponse struct {
	Id string `json:"id"`
}

type GetUsersRequest struct {
	Username string `query:"username,omitempty"`
	Limit    int    `query:"limit,omitempty"`
	Offset   int    `query:"offset,omitempty"`
}

type GetUsersResponse struct {
	Length int64       `json:"length"`
	Items  interface{} `json:"items"`
}

// Other models

type UserView struct {
	Id       *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Username string              `bson:"username,omitempty" json:"username,omitempty"`
	Email    string              `bson:"email,omitempty" json:"email,omitempty"`
	Password string              `bson:"password,omitempty" json:"-"`
	Picture  string              `bson:"picture,omitempty" json:"picture,omitempty"`
}

type UserDB struct {
	Id       *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Admin    bool                `bson:"admin,omitempty" json:"admin"`
	Username string              `bson:"username,omitempty" json:"username,omitempty"`
	Email    string              `bson:"email,omitempty" json:"email,omitempty"`
	Password string              `bson:"password,omitempty" json:"-"`
	Picture  string              `bson:"picture,omitempty" json:"picture,omitempty"`
}

type UserMe struct {
	Id       *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Username string              `bson:"username,omitempty" json:"username,omitempty"`
	Email    string              `bson:"email,omitempty" json:"email,omitempty"`
	Password string              `bson:"password,omitempty" json:"-"`
	Picture  string              `bson:"picture,omitempty" json:"picture,omitempty"`
}
