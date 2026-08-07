package jwt_manager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"recipes/internal/models"
)

func TestDecodeJWT(t *testing.T) {
	manager := JwtManager{AccessSecret: "access-secret", RefreshSecret: "refresh-secret"}
	_, token, err := manager.GenerateAccessJwt("user-id", false, time.Hour)
	require.NoError(t, err)

	t.Run("requires the expected purpose", func(t *testing.T) {
		_, err := DecodeJWT[models.AccessJwt](manager.AccessSecret, token, PurposeRefresh)
		assert.Error(t, err)

		decoded, err := DecodeJWT[models.AccessJwt](manager.AccessSecret, token, PurposeAccess)
		require.NoError(t, err)
		assert.Equal(t, "user-id", decoded.UserId)
		assert.Equal(t, PurposeAccess, decoded.Purpose)
	})

	t.Run("rejects a wrong secret", func(t *testing.T) {
		_, err := DecodeJWT[models.AccessJwt]("wrong-secret", token, PurposeAccess)
		assert.Error(t, err)
	})
}
