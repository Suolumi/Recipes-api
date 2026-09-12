package jwt_manager

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"recipes/internal/config"
	"recipes/internal/models"
	"time"
)

type JwtManager struct {
	AccessSecret      string
	AccessExpiration  time.Duration
	RefreshSecret     string
	RefreshExpiration time.Duration
	ResetSecret       string
	ResetExpiration   time.Duration
}

const (
	PurposeAccess  = "access"
	PurposeRefresh = "refresh"
	PurposeReset   = "reset"
)

func New(cfg *config.JwtConfig) *JwtManager {
	return &JwtManager{
		AccessSecret:      cfg.AccessSecret,
		AccessExpiration:  cfg.AccessExpiration,
		RefreshSecret:     cfg.RefreshSecret,
		RefreshExpiration: cfg.RefreshExpiration,
		ResetSecret:       cfg.ResetSecret,
		ResetExpiration:   cfg.ResetExpiration,
	}
}

func (m *JwtManager) secretFor(purpose string) (secret string, validity time.Duration, err error) {
	switch purpose {
	case PurposeAccess:
		return m.AccessSecret, m.AccessExpiration, nil
	case PurposeRefresh:
		return m.RefreshSecret, m.RefreshExpiration, nil
	case PurposeReset:
		return m.ResetSecret, m.ResetExpiration, nil
	default:
		return "", 0, fmt.Errorf("unknown token purpose %q", purpose)
	}
}

// Generate signs a token for the given purpose. The secret and lifetime are
// taken from the manager's configuration; admin is ignored for reset tokens by
// callers but stored regardless.
func (m *JwtManager) Generate(purpose, userID string, admin bool) (*models.TokenClaims, string, error) {
	secret, validity, err := m.secretFor(purpose)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	claims := &models.TokenClaims{
		UserId:  userID,
		Admin:   admin,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(validity)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	return claims, signed, err
}

func NewJwtClaims[T any](_ echo.Context) jwt.Claims {
	return any(new(T)).(jwt.Claims)
}

func GetJwt[T any](c echo.Context) T {
	user := c.Get("jwt").(*jwt.Token)
	claims := user.Claims.(T)

	return claims
}

func DecodeJWT[T any](secret, tokenString string, expectedPurpose ...string) (rt T, rerr error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected token signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return rt, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if len(expectedPurpose) > 0 {
			purpose, ok := claims["purpose"].(string)
			if !ok || purpose != expectedPurpose[0] {
				return rt, fmt.Errorf("invalid token purpose")
			}
		}
		bytes, err := json.Marshal(claims)
		if err != nil {
			return rt, err
		}
		if err = json.Unmarshal(bytes, &rt); err != nil {
			return rt, err
		}
		return rt, nil
	}

	return rt, fmt.Errorf("invalid token")
}
