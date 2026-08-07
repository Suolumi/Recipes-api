package jwt_manager

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
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

func (m *JwtManager) GenerateAccessJwt(id string, admin bool, validity time.Duration) (*models.AccessJwt, string, error) {
	claims := &models.AccessJwt{
		UserId:  id,
		Admin:   admin,
		Purpose: PurposeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(validity)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedString, err := token.SignedString([]byte(m.AccessSecret))
	return claims, signedString, err
}

func (m *JwtManager) GenerateRefreshJwt(id string, admin bool, validity time.Duration) (*models.RefreshJwt, string, error) {
	claims := &models.RefreshJwt{
		UserId:  id,
		Admin:   admin,
		Purpose: PurposeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(validity)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedString, err := token.SignedString([]byte(m.RefreshSecret))
	return claims, signedString, err
}

func (m *JwtManager) GenerateResetJwt(id string, validity time.Duration) (*models.ResetJwt, string, error) {
	claims := &models.ResetJwt{
		UserId:  id,
		Purpose: PurposeReset,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(validity)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedString, err := token.SignedString([]byte(m.ResetSecret))
	return claims, signedString, err
}
