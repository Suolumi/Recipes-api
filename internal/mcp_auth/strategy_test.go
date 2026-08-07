package mcp_auth

import (
	"context"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
)

func TestJWTAccessStrategy(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	config := &fosite.Config{GlobalSecret: secret, AccessTokenLifespan: time.Hour}
	strategy := NewJWTAccessStrategy(compose.NewOAuth2HMACStrategy(config), secret, "https://recipes.example", "https://recipes.example/mcp")
	session := &fosite.DefaultSession{Subject: "user-id", Extra: map[string]any{"auth_version": int64(4)}}
	session.SetExpiresAt(fosite.AccessToken, time.Now().Add(time.Hour))
	request := &fosite.Request{
		ID: "request-id", Client: &fosite.DefaultClient{ID: "https://agent.example/client.json"},
		GrantedScope: fosite.Arguments{"recipes:read"}, Session: session,
	}
	token, signature, err := strategy.GenerateAccessToken(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if signature != tokenSignature(token) {
		t.Fatal("access-token signature is not deterministic")
	}
	claims, err := ParseAccessToken(token, secret, "https://recipes.example", "https://recipes.example/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-id" || claims.ClientID != "https://agent.example/client.json" || claims.AuthVersion != 4 || !HasScope(claims, "recipes:read") {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if _, err := ParseAccessToken(token, secret, "https://recipes.example", "https://other.example/mcp"); err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
}
