package mcp_server

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/models"
)

const testSecret = "test-secret-0123456789abcdefghij"

type fakeUsers struct {
	user models.UserDB
	err  error
}

func (f fakeUsers) GetUserById(string) (models.UserDB, error) { return f.user, f.err }

func userAtVersion(v int64) models.UserDB {
	id := primitive.NewObjectID()
	return models.UserDB{Id: &id, MCPAuthVersion: v}
}

func TestAuthenticatorRoundTrip(t *testing.T) {
	user := userAtVersion(3)
	a := NewAuthenticator(fakeUsers{user: user}, testSecret, time.Hour)

	token, err := a.Mint(user.Id.Hex())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	info, err := a.Verifier()(context.Background(), token, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info.UserID != user.Id.Hex() {
		t.Fatalf("UserID = %q, want %q", info.UserID, user.Id.Hex())
	}
	if !slices.Contains(info.Scopes, "recipes:read") || !slices.Contains(info.Scopes, "recipes:write") {
		t.Fatalf("Scopes = %v", info.Scopes)
	}
	if info.Expiration.IsZero() {
		t.Fatal("Expiration not set")
	}
}

func TestAuthenticatorRejects(t *testing.T) {
	user := userAtVersion(3)
	issuer := NewAuthenticator(fakeUsers{user: user}, testSecret, time.Hour)
	token, err := issuer.Mint(user.Id.Hex())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	assertRejected := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, auth.ErrInvalidToken) {
			t.Fatalf("err = %v, want auth.ErrInvalidToken", err)
		}
	}

	t.Run("wrong secret", func(t *testing.T) {
		other := NewAuthenticator(fakeUsers{user: user}, "another-secret-0123456789abcdef", time.Hour)
		_, err := other.Verifier()(context.Background(), token, nil)
		assertRejected(t, err)
	})

	t.Run("expired", func(t *testing.T) {
		expired, err := NewAuthenticator(fakeUsers{user: user}, testSecret, -time.Hour).Mint(user.Id.Hex())
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		_, err = issuer.Verifier()(context.Background(), expired, nil)
		assertRejected(t, err)
	})

	t.Run("auth version mismatch", func(t *testing.T) {
		stale := NewAuthenticator(fakeUsers{user: userAtVersion(4)}, testSecret, time.Hour)
		_, err := stale.Verifier()(context.Background(), token, nil)
		assertRejected(t, err)
	})

	t.Run("deleted user", func(t *testing.T) {
		gone := NewAuthenticator(fakeUsers{err: mongorepo.UserNotFoundError}, testSecret, time.Hour)
		_, err := gone.Verifier()(context.Background(), token, nil)
		assertRejected(t, err)
	})

	t.Run("wrong purpose", func(t *testing.T) {
		raw := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, mcpClaims{
			Purpose:     "access",
			AuthVersion: 3,
			RegisteredClaims: jwtlib.RegisteredClaims{
				Subject:   user.Id.Hex(),
				ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		signed, err := raw.SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = issuer.Verifier()(context.Background(), signed, nil)
		assertRejected(t, err)
	})

	t.Run("no expiration claim", func(t *testing.T) {
		raw := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, mcpClaims{
			Purpose:          purposeMCP,
			AuthVersion:      3,
			RegisteredClaims: jwtlib.RegisteredClaims{Subject: user.Id.Hex()},
		})
		signed, err := raw.SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = issuer.Verifier()(context.Background(), signed, nil)
		assertRejected(t, err)
	})
}
