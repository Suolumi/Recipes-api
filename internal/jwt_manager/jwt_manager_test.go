package jwt_manager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"recipes/internal/config"
	"recipes/internal/models"
)

func testManager() *JwtManager {
	return &JwtManager{
		AccessSecret:      "access-secret",
		AccessExpiration:  time.Hour,
		RefreshSecret:     "refresh-secret",
		RefreshExpiration: time.Hour,
		ResetSecret:       "reset-secret",
		ResetExpiration:   time.Hour,
	}
}

func TestGenerateAndDecode(t *testing.T) {
	m := testManager()

	for _, tc := range []struct{ purpose, secret string }{
		{PurposeAccess, m.AccessSecret},
		{PurposeRefresh, m.RefreshSecret},
		{PurposeReset, m.ResetSecret},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			claims, token, err := m.Generate(tc.purpose, "user-id", true)
			require.NoError(t, err)
			assert.Equal(t, tc.purpose, claims.Purpose)
			require.NotNil(t, claims.ExpiresAt)

			decoded, err := DecodeJWT[models.TokenClaims](tc.secret, token, tc.purpose)
			require.NoError(t, err)
			assert.Equal(t, "user-id", decoded.UserId)
			assert.Equal(t, tc.purpose, decoded.Purpose)

			_, err = DecodeJWT[models.TokenClaims](tc.secret, token, "some-other-purpose")
			assert.Error(t, err, "wrong expected purpose must be rejected")

			_, err = DecodeJWT[models.TokenClaims]("wrong-secret", token, tc.purpose)
			assert.Error(t, err, "wrong secret must be rejected")
		})
	}
}

func TestGenerateUnknownPurpose(t *testing.T) {
	_, _, err := testManager().Generate("nonsense", "user-id", false)
	assert.Error(t, err)
}

func TestNewCopiesConfig(t *testing.T) {
	m := New(&config.JwtConfig{
		AccessSecret:      "s-access",
		AccessExpiration:  15 * time.Minute,
		RefreshSecret:     "s-refresh",
		RefreshExpiration: 48 * time.Hour,
		ResetSecret:       "s-reset",
		ResetExpiration:   time.Hour,
	})
	assert.Equal(t, "s-access", m.AccessSecret)
	assert.Equal(t, 15*time.Minute, m.AccessExpiration)
	assert.Equal(t, "s-refresh", m.RefreshSecret)
	assert.Equal(t, 48*time.Hour, m.RefreshExpiration)
	assert.Equal(t, "s-reset", m.ResetSecret)
	assert.Equal(t, time.Hour, m.ResetExpiration)
}
