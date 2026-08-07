package mcp_auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
)

type AccessClaims struct {
	ClientID    string `json:"client_id"`
	Scope       string `json:"scope"`
	AuthVersion int64  `json:"auth_version"`
	jwtlib.RegisteredClaims
}

type JWTAccessStrategy struct {
	oauth2.CoreStrategy
	secret   []byte
	issuer   string
	audience string
}

func NewJWTAccessStrategy(core oauth2.CoreStrategy, secret []byte, issuer, audience string) *JWTAccessStrategy {
	return &JWTAccessStrategy{CoreStrategy: core, secret: secret, issuer: issuer, audience: audience}
}

func tokenSignature(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *JWTAccessStrategy) AccessTokenSignature(_ context.Context, token string) string {
	return tokenSignature(token)
}

func authVersion(session fosite.Session) int64 {
	extra, ok := session.(*fosite.DefaultSession)
	if !ok || extra.Extra == nil {
		return 0
	}
	switch value := extra.Extra["auth_version"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func (s *JWTAccessStrategy) GenerateAccessToken(_ context.Context, requester fosite.Requester) (string, string, error) {
	expires := requester.GetSession().GetExpiresAt(fosite.AccessToken)
	if expires.IsZero() {
		expires = time.Now().Add(time.Hour)
	}
	claims := AccessClaims{
		ClientID: requester.GetClient().GetID(), Scope: strings.Join(requester.GetGrantedScopes(), " "),
		AuthVersion: authVersion(requester.GetSession()),
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer: s.issuer, Subject: requester.GetSession().GetSubject(), Audience: jwtlib.ClaimStrings{s.audience},
			ExpiresAt: jwtlib.NewNumericDate(expires), IssuedAt: jwtlib.NewNumericDate(time.Now()), ID: requester.GetID(),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, tokenSignature(signed), err
}

func (s *JWTAccessStrategy) ValidateAccessToken(_ context.Context, _ fosite.Requester, token string) error {
	_, err := ParseAccessToken(token, s.secret, s.issuer, s.audience)
	return err
}

func ParseAccessToken(raw string, secret []byte, issuer, audience string) (*AccessClaims, error) {
	claims := new(AccessClaims)
	token, err := jwtlib.ParseWithClaims(raw, claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, errors.New("unexpected access token signing method")
		}
		return secret, nil
	}, jwtlib.WithIssuer(issuer), jwtlib.WithAudience(audience), jwtlib.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" || claims.ClientID == "" {
		return nil, auth.ErrInvalidToken
	}
	return claims, nil
}

func HasScope(claims *AccessClaims, scope string) bool {
	return slicesContains(strings.Fields(claims.Scope), scope)
}

func slicesContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
