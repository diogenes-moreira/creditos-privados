package model_test

import (
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name        string
		firebaseUID string
		email       string
		role        model.UserRole
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid admin user",
			firebaseUID: "firebase-uid-123",
			email:       "admin@example.com",
			role:        model.RoleAdmin,
		},
		{
			name:        "valid client user",
			firebaseUID: "firebase-uid-456",
			email:       "client@example.com",
			role:        model.RoleClient,
		},
		{
			name:        "empty firebase UID",
			firebaseUID: "",
			email:       "test@example.com",
			role:        model.RoleClient,
			wantErr:     true,
			errMsg:      "firebase UID is required",
		},
		{
			name:        "empty email",
			firebaseUID: "firebase-uid-789",
			email:       "",
			role:        model.RoleClient,
			wantErr:     true,
			errMsg:      "email is required",
		},
		{
			name:        "invalid role",
			firebaseUID: "firebase-uid-101",
			email:       "test@example.com",
			role:        model.UserRole("superadmin"),
			wantErr:     true,
			errMsg:      "invalid role: superadmin",
		},
		{
			name:        "empty role string",
			firebaseUID: "firebase-uid-102",
			email:       "test@example.com",
			role:        model.UserRole(""),
			wantErr:     true,
			errMsg:      "invalid role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := model.NewUser(tt.firebaseUID, tt.email, tt.role)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				require.NotNil(t, user)
				assert.NotEqual(t, user.ID.String(), "00000000-0000-0000-0000-000000000000")
				assert.Equal(t, tt.firebaseUID, user.FirebaseUID)
				assert.Equal(t, tt.email, user.Email)
				assert.Equal(t, tt.role, user.Role)
				assert.True(t, user.IsActive)
				assert.Nil(t, user.LastLoginAt)
			}
		})
	}
}

func TestUser_Activate(t *testing.T) {
	user, err := model.NewUser("uid", "test@example.com", model.RoleClient)
	require.NoError(t, err)

	user.Deactivate()
	assert.False(t, user.IsActive)

	user.Activate()
	assert.True(t, user.IsActive)
}

func TestUser_Deactivate(t *testing.T) {
	user, err := model.NewUser("uid", "test@example.com", model.RoleClient)
	require.NoError(t, err)
	assert.True(t, user.IsActive)

	user.Deactivate()
	assert.False(t, user.IsActive)

	// Deactivating again should stay false
	user.Deactivate()
	assert.False(t, user.IsActive)
}

func TestUser_IsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		role     model.UserRole
		expected bool
	}{
		{name: "admin role returns true", role: model.RoleAdmin, expected: true},
		{name: "client role returns false", role: model.RoleClient, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := model.NewUser("uid", "test@example.com", tt.role)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, user.IsAdmin())
		})
	}
}

func TestUser_RecordLogin(t *testing.T) {
	user, err := model.NewUser("uid", "test@example.com", model.RoleClient)
	require.NoError(t, err)
	assert.Nil(t, user.LastLoginAt)

	user.RecordLogin()
	require.NotNil(t, user.LastLoginAt)
}

func TestUser_SetPhone(t *testing.T) {
	user, err := model.NewUser("uid", "test@example.com", model.RoleClient)
	require.NoError(t, err)
	assert.Empty(t, user.Phone)

	user.SetPhone("+5491140510100")
	assert.Equal(t, "+5491140510100", user.Phone)

	user.SetPhone("+5491199999999")
	assert.Equal(t, "+5491199999999", user.Phone)
}

func TestUser_IsVendor(t *testing.T) {
	user, err := model.NewUser("uid", "test@example.com", model.RoleVendor)
	require.NoError(t, err)
	assert.True(t, user.IsVendor())

	clientUser, err := model.NewUser("uid2", "client@example.com", model.RoleClient)
	require.NoError(t, err)
	assert.False(t, clientUser.IsVendor())
}

func TestUserChangeRole(t *testing.T) {
	newUser := func(t *testing.T) *model.User {
		t.Helper()
		u, err := model.NewUser("uid-1", "ana@example.com", model.RoleClient)
		require.NoError(t, err)
		return u
	}

	t.Run("moves between known roles", func(t *testing.T) {
		u := newUser(t)

		require.NoError(t, u.ChangeRole(model.RoleAdmin))
		assert.Equal(t, model.RoleAdmin, u.Role)
		assert.True(t, u.IsAdmin())

		require.NoError(t, u.ChangeRole(model.RoleVendor))
		assert.True(t, u.IsVendor())
		assert.False(t, u.IsAdmin())
	})

	t.Run("rejects an unknown role and leaves the user untouched", func(t *testing.T) {
		u := newUser(t)

		err := u.ChangeRole("superuser")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
		// The middleware compares roles by value, so a stored unknown role
		// would silently fail every authorization check.
		assert.Equal(t, model.RoleClient, u.Role)
	})

	t.Run("changing role does not alter activation", func(t *testing.T) {
		u := newUser(t)
		u.Deactivate()

		require.NoError(t, u.ChangeRole(model.RoleAdmin))

		assert.False(t, u.IsActive, "a promoted user must not be reactivated implicitly")
	})
}

func TestValidRole(t *testing.T) {
	for _, r := range []model.UserRole{model.RoleAdmin, model.RoleClient, model.RoleVendor} {
		assert.True(t, model.ValidRole(r), string(r))
	}
	for _, r := range []model.UserRole{"", "superuser", "Admin"} {
		assert.False(t, model.ValidRole(r), string(r))
	}
}
