package mongo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"recipes/internal/models"
	"recipes/internal/utils"
)

const userCollection = "users"

var UserNotFoundError = errors.New("user not found")
var UserConflictError = errors.New("user conflict")

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
		return models.UserDB{}, UserNotFoundError
	} else if err != nil {
		return models.UserDB{}, err
	}
	return user, nil
}

func (c *Client) GetUserByIdentifier(identifier string) (models.UserDB, error) {
	var user models.UserDB
	filter := bson.M{"$or": []bson.M{
		{"username": identifier},
		{"email": identifier},
	}}
	err := c.db.Collection(userCollection).FindOne(context.Background(), filter).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, UserNotFoundError
	}
	return user, err
}

func (c *Client) UpdateUserById(id string, user models.UserDB) (models.UserDB, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.UserDB{}, err
	}

	passwordChanged := user.Password != ""
	if passwordChanged {
		hashedPassword, err := utils.HashPassword(user.Password)
		if err != nil {
			return models.UserDB{}, err
		}

		user.Password = hashedPassword
	}

	update := bson.M{"$set": user}
	if passwordChanged {
		update["$inc"] = bson.M{"mcp_auth_version": 1}
	}
	cursor := c.db.Collection(userCollection).FindOneAndUpdate(context.TODO(), bson.M{
		"_id": objectId,
	}, update)
	var updated models.UserDB
	if err := cursor.Decode(&updated); errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, UserNotFoundError
	} else if err != nil {
		return models.UserDB{}, err
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
	var updated models.UserDB
	if err := cursor.Decode(&updated); errors.Is(err, mongo.ErrNoDocuments) {
		return models.UserDB{}, UserNotFoundError
	} else if err != nil {
		return models.UserDB{}, err
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
		return user, UserNotFoundError
	} else if err != nil {
		return user, err
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

	filter := bson.M{}
	if username != "" {
		filter["username"] = primitive.Regex{Pattern: regexp.QuoteMeta(username), Options: "i"}
	}
	number, err := c.db.Collection(userCollection).CountDocuments(context.TODO(), filter)
	if err != nil {
		return nil, 0, err
	}

	cursor, err := c.db.Collection(userCollection).Find(context.TODO(), filter, reqOptions)
	if err != nil {
		return nil, 0, err
	}

	if err = cursor.All(context.TODO(), &users); err != nil {
		return nil, 0, err
	}

	return users, number, nil
}

func (c *Client) CreateUser(user models.UserDB) (models.UserDB, error) {
	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		return models.UserDB{}, err
	}

	user.Password = hashedPassword

	cursor, err := c.db.Collection(userCollection).InsertOne(context.TODO(), user)
	if err != nil {
		return models.UserDB{}, err
	}
	return c.GetUserById(cursor.InsertedID.(primitive.ObjectID).Hex())
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

		cursor := c.db.Collection(userCollection).FindOne(context.TODO(), bson.M{bsonName: field.Interface()})
		if err := cursor.Decode(&dbUser); err == nil {
			return dbUser, fmt.Errorf("%w: %s is already taken", UserConflictError, bsonName)
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return models.UserDB{}, fmt.Errorf("check user conflict: %w", err)
		}

	}

	return models.UserDB{}, nil
}
