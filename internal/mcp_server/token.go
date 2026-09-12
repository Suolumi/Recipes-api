package mcp_server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"recipes/internal/models"
)

const purposeMCP = "mcp"

// userLookup is the slice of the database the Authenticator needs. Anything that
// can resolve a user by id (including database.Database) satisfies it.
type userLookup interface {
	GetUserById(id string) (models.UserDB, error)
}

// mcpClaims is the payload of an MCP access token. It is a plain HS256 JWT: the
// user id lives in the standard Subject claim, AuthVersion pins the token to the
// user's MCPAuthVersion so a password change (which increments it) invalidates
// every outstanding token.
type mcpClaims struct {
	Purpose     string `json:"purpose"`
	AuthVersion int64  `json:"mcp_ver"`
	jwtlib.RegisteredClaims
}

// Authenticator mints and verifies MCP access tokens. Tokens are stateless: the
// only server-side state consulted on verification is the user record.
type Authenticator struct {
	users  userLookup
	secret []byte
	ttl    time.Duration
}

func NewAuthenticator(users userLookup, secret string, ttl time.Duration) *Authenticator {
	return &Authenticator{users: users, secret: []byte(secret), ttl: ttl}
}

// Mint issues a token for userID, valid for the configured TTL.
func (a *Authenticator) Mint(userID string) (string, error) {
	user, err := a.users.GetUserById(userID)
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := mcpClaims{
		Purpose:     purposeMCP,
		AuthVersion: user.MCPAuthVersion,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(a.ttl)),
		},
	}
	return jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(a.secret)
}

// Verifier returns a TokenVerifier for the MCP SDK's RequireBearerToken
// middleware. Every rejection unwraps to auth.ErrInvalidToken (→ 401).
func (a *Authenticator) Verifier() auth.TokenVerifier {
	return func(_ context.Context, raw string, _ *http.Request) (*auth.TokenInfo, error) {
		token, err := jwtlib.ParseWithClaims(raw, &mcpClaims{}, func(t *jwtlib.Token) (any, error) {
			if t.Method != jwtlib.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return a.secret, nil
		}, jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}), jwtlib.WithExpirationRequired())
		if err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		claims, ok := token.Claims.(*mcpClaims)
		if !ok || !token.Valid || claims.Purpose != purposeMCP || claims.Subject == "" || claims.ExpiresAt == nil {
			return nil, fmt.Errorf("%w: malformed MCP token", auth.ErrInvalidToken)
		}
		user, err := a.users.GetUserById(claims.Subject)
		if err != nil || user.MCPAuthVersion != claims.AuthVersion {
			return nil, fmt.Errorf("%w: token no longer valid", auth.ErrInvalidToken)
		}
		return &auth.TokenInfo{
			UserID:     claims.Subject,
			Scopes:     []string{"recipes:read", "recipes:write"},
			Expiration: claims.ExpiresAt.Time,
		}, nil
	}
}
