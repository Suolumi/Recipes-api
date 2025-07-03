package mongo

import (
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/net/context"
	"recipes/internal/models"
	"recipes/internal/utils"
	"reflect"
	"strings"
)

const userCollection = "users"

func (c *Client) GetUserById(id string) (models.UserDB, error) {
	var user models.UserDB

	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.UserDB{}, err
	}
	cursor := c.db.Collection(userCollection).FindOne(context.TODO(), bson.M{
		"_id": objectId,
	})
	if err := cursor.Decode(&user); errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, fmt.Errorf("user not found")
	}
	return user, nil
}

func (c *Client) UpdateUserById(id string, user models.UserDB) (models.UserDB, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.UserDB{}, err
	}

	if user.Password != "" {
		hashedPassword, err := utils.HashPassword(user.Password)
		if err != nil {
			return models.UserDB{}, err
		}

		user.Password = hashedPassword
	}

	cursor := c.db.Collection(userCollection).FindOneAndUpdate(context.TODO(), bson.M{
		"_id": objectId,
	}, bson.M{
		"$set": user,
	})
	if err := cursor.Decode(&user); errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, fmt.Errorf("user not found")
	}
	return c.GetUserById(id)
}

func (c *Client) UpdateUserInterfaceById(id string, user interface{}) (models.UserDB, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.UserDB{}, err
	}

	cursor := c.db.Collection(userCollection).FindOneAndUpdate(context.TODO(), bson.M{
		"_id": objectId,
	}, bson.M{
		"$set": user,
	})
	if err := cursor.Decode(&user); errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, fmt.Errorf("user not found")
	}
	return c.GetUserById(id)
}

func (c *Client) DeleteUserById(id string) (models.UserDB, error) {
	var user models.UserDB
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return user, err
	}

	cursor := c.db.Collection(userCollection).FindOneAndDelete(context.TODO(), bson.M{
		"_id": objectId,
	})
	if err := cursor.Decode(&user); errors.Is(err, mongo.ErrNoDocuments) {
		return user, fmt.Errorf("user not found")
	}
	return user, nil
}

func (c *Client) GetUsers(username string, limit, offset int) ([]models.UserDB, int64, error) {
	var users []models.UserDB
	reqOptions := options.Find()

	if limit != 0 {
		reqOptions.SetLimit(int64(limit))
	}
	if offset != 0 {
		reqOptions.SetSkip(int64(offset))
	}

	number, err := c.db.Collection(userCollection).CountDocuments(context.TODO(), bson.M{})
	if err != nil {
		return nil, 0, err
	}

	cursor, err := c.db.Collection(userCollection).Find(context.TODO(), bson.M{
		"username": primitive.Regex{Pattern: username, Options: "i"},
	}, reqOptions)
	if err != nil {
		return nil, 0, err
	}

	if err = cursor.All(context.TODO(), &users); err != nil {
		return nil, 0, err
	}

	return users, number, nil
}

func (c *Client) CreateUser(user models.UserDB) (string, error) {
	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		return "", err
	}

	user.Password = hashedPassword

	cursor, err := c.db.Collection(userCollection).InsertOne(context.TODO(), user)
	if err != nil {
		return "", err
	}
	return cursor.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (c *Client) UserConflicts(user models.UserDB) (models.UserDB, error) {
	val := reflect.ValueOf(&user).Elem()
	typ := val.Type()
	dbUser := models.UserDB{}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldInfos := typ.Field(i)

		if field.IsZero() {
			continue
		}

		bsonName := strings.Split(fieldInfos.Tag.Get("bson"), ",")[0]

		if cursor := c.db.Collection(userCollection).FindOne(context.TODO(), bson.M{bsonName: field.Interface()}); !errors.Is(cursor.Decode(&dbUser), mongo.ErrNoDocuments) {
			return dbUser, fmt.Errorf("%s is already taken", bsonName)
		}

	}

	return models.UserDB{}, nil
}
