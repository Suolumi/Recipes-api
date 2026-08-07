package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const idempotencyCollection = "mcp_idempotency"

var (
	ErrConflict = errors.New("idempotency key was already used with different input")
	ErrPending  = errors.New("an operation with this idempotency key is already in progress")
	ErrMissing  = errors.New("idempotency record no longer exists")
)

type idempotencyDocument struct {
	ID          string          `bson:"_id"`
	PayloadHash string          `bson:"payload_hash"`
	Response    json.RawMessage `bson:"response,omitempty"`
	ExpiresAt   time.Time       `bson:"expires_at"`
}

type Store struct {
	collection *mongo.Collection
	ttl        time.Duration
}

func New(ctx context.Context, db *mongo.Database, ttl time.Duration) (*Store, error) {
	collection := db.Collection(idempotencyCollection)
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)})
	if err != nil {
		return nil, err
	}
	return &Store{collection: collection, ttl: ttl}, nil
}

func payloadHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func (s *Store) Reserve(ctx context.Context, userID, operation, key string, input any, output any) (bool, error) {
	hash, err := payloadHash(input)
	if err != nil {
		return false, err
	}
	id := userID + ":" + operation + ":" + key
	document := idempotencyDocument{ID: id, PayloadHash: hash, ExpiresAt: time.Now().Add(s.ttl)}
	_, err = s.collection.InsertOne(ctx, document)
	if err == nil {
		return false, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return false, err
	}
	if err := s.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&document); err != nil {
		return false, err
	}
	if document.PayloadHash != hash {
		return false, ErrConflict
	}
	if len(document.Response) == 0 {
		return false, ErrPending
	}
	if err := json.Unmarshal(document.Response, output); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) Complete(ctx context.Context, userID, operation, key string, response any) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	result, err := s.collection.UpdateOne(ctx, bson.M{"_id": userID + ":" + operation + ":" + key}, bson.M{"$set": bson.M{"response": data}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrMissing
	}
	return nil
}

func (s *Store) Abandon(ctx context.Context, userID, operation, key string) {
	_, _ = s.collection.DeleteOne(ctx, bson.M{"_id": userID + ":" + operation + ":" + key, "response": bson.M{"$exists": false}})
}
