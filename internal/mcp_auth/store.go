package mcp_auth

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/ory/fosite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const oauthSessionsCollection = "mcp_oauth_sessions"

type requestDocument struct {
	ID                string                 `bson:"_id"`
	Kind              string                 `bson:"kind"`
	RequestID         string                 `bson:"request_id"`
	RequestedAt       time.Time              `bson:"requested_at"`
	Client            *fosite.DefaultClient  `bson:"client"`
	RequestedScope    []string               `bson:"requested_scope"`
	GrantedScope      []string               `bson:"granted_scope"`
	Form              map[string][]string    `bson:"form"`
	RequestedAudience []string               `bson:"requested_audience"`
	GrantedAudience   []string               `bson:"granted_audience"`
	Session           *fosite.DefaultSession `bson:"session"`
	Active            bool                   `bson:"active"`
	AccessSignature   string                 `bson:"access_signature,omitempty"`
	ExpiresAt         time.Time              `bson:"expires_at"`
}

type ClientResolver interface {
	Resolve(context.Context, string) (*ClientMetadata, *fosite.DefaultClient, error)
}

type MongoStore struct {
	collection *mongo.Collection
	clients    ClientResolver
	codeTTL    time.Duration
}

func NewMongoStore(ctx context.Context, db *mongo.Database, clients ClientResolver, codeTTL time.Duration) (*MongoStore, error) {
	collection := db.Collection(oauthSessionsCollection)
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)})
	if err != nil {
		return nil, err
	}
	_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "request_id", Value: 1}, {Key: "kind", Value: 1}}})
	if err != nil {
		return nil, err
	}
	return &MongoStore{collection: collection, clients: clients, codeTTL: codeTTL}, nil
}

func requestToDocument(kind, signature string, requester fosite.Requester, expiresAt time.Time) (*requestDocument, error) {
	client := requester.GetClient()
	defaultClient := &fosite.DefaultClient{
		ID: client.GetID(), RedirectURIs: client.GetRedirectURIs(), GrantTypes: client.GetGrantTypes(),
		ResponseTypes: client.GetResponseTypes(), Scopes: client.GetScopes(), Audience: client.GetAudience(), Public: client.IsPublic(),
	}
	session, ok := requester.GetSession().(*fosite.DefaultSession)
	if !ok {
		return nil, errors.New("oauth session has an unsupported type")
	}
	return &requestDocument{
		ID: kind + ":" + signature, Kind: kind, RequestID: requester.GetID(), RequestedAt: requester.GetRequestedAt(),
		Client: defaultClient, RequestedScope: requester.GetRequestedScopes(), GrantedScope: requester.GetGrantedScopes(),
		Form: requester.GetRequestForm(), RequestedAudience: requester.GetRequestedAudience(), GrantedAudience: requester.GetGrantedAudience(),
		Session: session.Clone().(*fosite.DefaultSession), Active: true, ExpiresAt: expiresAt,
	}, nil
}

func (d *requestDocument) requester() fosite.Requester {
	form := url.Values{}
	for key, values := range d.Form {
		form[key] = values
	}
	return &fosite.Request{
		ID: d.RequestID, RequestedAt: d.RequestedAt, Client: d.Client,
		RequestedScope: d.RequestedScope, GrantedScope: d.GrantedScope, Form: form,
		Session: d.Session, RequestedAudience: d.RequestedAudience, GrantedAudience: d.GrantedAudience,
	}
}

func (s *MongoStore) create(ctx context.Context, kind, signature string, requester fosite.Requester, expiresAt time.Time) error {
	document, err := requestToDocument(kind, signature, requester, expiresAt)
	if err != nil {
		return err
	}
	_, err = s.collection.InsertOne(ctx, document)
	return err
}

func (s *MongoStore) get(ctx context.Context, kind, signature string) (*requestDocument, error) {
	var document requestDocument
	err := s.collection.FindOne(ctx, bson.M{"_id": kind + ":" + signature}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fosite.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &document, nil
}

func (s *MongoStore) delete(ctx context.Context, kind, signature string) error {
	_, err := s.collection.DeleteOne(ctx, bson.M{"_id": kind + ":" + signature})
	return err
}

func (s *MongoStore) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	_, client, err := s.clients.Resolve(ctx, id)
	if err != nil {
		return nil, fosite.ErrNotFound.WithWrap(err)
	}
	return client, nil
}

func (s *MongoStore) ClientAssertionJWTValid(context.Context, string) error          { return nil }
func (s *MongoStore) SetClientAssertionJWT(context.Context, string, time.Time) error { return nil }

func (s *MongoStore) CreateAuthorizeCodeSession(ctx context.Context, signature string, requester fosite.Requester) error {
	return s.create(ctx, "code", signature, requester, time.Now().Add(s.codeTTL))
}

func (s *MongoStore) GetAuthorizeCodeSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	document, err := s.get(ctx, "code", signature)
	if err != nil {
		return nil, err
	}
	if !document.Active {
		return document.requester(), fosite.ErrInvalidatedAuthorizeCode
	}
	return document.requester(), nil
}

func (s *MongoStore) InvalidateAuthorizeCodeSession(ctx context.Context, signature string) error {
	result, err := s.collection.UpdateOne(ctx, bson.M{"_id": "code:" + signature, "active": true}, bson.M{"$set": bson.M{"active": false}})
	if err == nil && result.MatchedCount == 0 {
		return fosite.ErrNotFound
	}
	return err
}

func (s *MongoStore) CreatePKCERequestSession(ctx context.Context, signature string, requester fosite.Requester) error {
	return s.create(ctx, "pkce", signature, requester, time.Now().Add(s.codeTTL))
}

func (s *MongoStore) GetPKCERequestSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	document, err := s.get(ctx, "pkce", signature)
	if err != nil {
		return nil, err
	}
	return document.requester(), nil
}

func (s *MongoStore) DeletePKCERequestSession(ctx context.Context, signature string) error {
	return s.delete(ctx, "pkce", signature)
}

func (s *MongoStore) CreateAccessTokenSession(ctx context.Context, signature string, requester fosite.Requester) error {
	return nil
}

func (s *MongoStore) GetAccessTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return nil, fosite.ErrNotFound
}

func (s *MongoStore) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	return nil
}

func (s *MongoStore) CreateRefreshTokenSession(ctx context.Context, signature, accessSignature string, requester fosite.Requester) error {
	expires := requester.GetSession().GetExpiresAt(fosite.RefreshToken)
	document, err := requestToDocument("refresh", signature, requester, expires)
	if err != nil {
		return err
	}
	document.AccessSignature = accessSignature
	_, err = s.collection.InsertOne(ctx, document)
	return err
}

func (s *MongoStore) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	document, err := s.get(ctx, "refresh", signature)
	if err != nil {
		return nil, err
	}
	if !document.Active {
		return document.requester(), fosite.ErrInactiveToken
	}
	return document.requester(), nil
}

func (s *MongoStore) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	return s.delete(ctx, "refresh", signature)
}

func (s *MongoStore) RotateRefreshToken(ctx context.Context, requestID, refreshSignature string) error {
	_, err := s.collection.UpdateMany(ctx, bson.M{"request_id": requestID, "kind": "refresh"}, bson.M{"$set": bson.M{"active": false}})
	if err != nil {
		return err
	}
	return s.RevokeAccessToken(ctx, requestID)
}

func (s *MongoStore) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, err := s.collection.UpdateMany(ctx, bson.M{"request_id": requestID, "kind": "refresh"}, bson.M{"$set": bson.M{"active": false}})
	return err
}

func (s *MongoStore) RevokeAccessToken(ctx context.Context, requestID string) error {
	return nil
}
