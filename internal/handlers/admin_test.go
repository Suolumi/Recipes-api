package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"recipes/internal/config"
	"recipes/internal/models"
)

func TestEvaluateAdminDemotion(t *testing.T) {
	dbCfg := &config.DatabaseConfig{AdminUsername: "admin", AdminMail: "admin@admin.admin"}

	callerID := primitive.NewObjectID()
	targetID := primitive.NewObjectID()
	otherAdminID := primitive.NewObjectID()

	target := models.UserDB{Id: &targetID, Username: "someone", Email: "someone@example.test", Admin: true}

	t.Run("blocks self-demotion", func(t *testing.T) {
		self := models.UserDB{Id: &callerID, Username: "caller", Email: "caller@example.test", Admin: true}
		err := evaluateAdminDemotion(callerID.Hex(), self, dbCfg, 3)
		assert.ErrorIs(t, err, ErrSelfDemotion)
	})

	t.Run("blocks demoting the bootstrap admin by username", func(t *testing.T) {
		bootstrap := models.UserDB{Id: &otherAdminID, Username: "admin", Email: "someone-else@example.test", Admin: true}
		err := evaluateAdminDemotion(callerID.Hex(), bootstrap, dbCfg, 3)
		assert.ErrorIs(t, err, ErrBootstrapAdminDemotion)
	})

	t.Run("blocks demoting the bootstrap admin by email", func(t *testing.T) {
		bootstrap := models.UserDB{Id: &otherAdminID, Username: "renamed-admin", Email: "admin@admin.admin", Admin: true}
		err := evaluateAdminDemotion(callerID.Hex(), bootstrap, dbCfg, 3)
		assert.ErrorIs(t, err, ErrBootstrapAdminDemotion)
	})

	t.Run("blocks demoting the last admin", func(t *testing.T) {
		err := evaluateAdminDemotion(callerID.Hex(), target, dbCfg, 1)
		assert.ErrorIs(t, err, ErrLastAdminDemotion)
	})

	t.Run("allows demoting an ordinary admin when others remain", func(t *testing.T) {
		err := evaluateAdminDemotion(callerID.Hex(), target, dbCfg, 2)
		assert.NoError(t, err)
	})
}
