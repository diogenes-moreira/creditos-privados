package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRole string

const (
	RoleAdmin  UserRole = "admin"
	RoleClient UserRole = "client"
	RoleVendor UserRole = "vendor"
)

type User struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`
	// FirebaseUID keeps its column name for continuity: it is simply the
	// external identity id, now generated locally since sign-in is a one-time
	// code rather than a third-party provider.
	FirebaseUID string         `gorm:"uniqueIndex;not null"`
	Email       string         `gorm:"uniqueIndex;not null"`
	Phone       string         `gorm:"index"`
	Role        UserRole       `gorm:"type:varchar(20);not null;default:'client'"`
	IsActive    bool           `gorm:"not null;default:true"`
	LastLoginAt *time.Time     `gorm:""`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func NewUser(firebaseUID, email string, role UserRole) (*User, error) {
	if firebaseUID == "" {
		return nil, fmt.Errorf("firebase UID is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if role != RoleAdmin && role != RoleClient && role != RoleVendor {
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	return &User{
		ID:          uuid.New(),
		FirebaseUID: firebaseUID,
		Email:       email,
		Role:        role,
		IsActive:    true,
	}, nil
}

// ValidRole reports whether a role is one the system knows.
func ValidRole(role UserRole) bool {
	return role == RoleAdmin || role == RoleClient || role == RoleVendor
}

// ChangeRole moves the user to another role.
//
// Role changes are deliberately not silent: an unknown role is rejected rather
// than stored, because the authorization middleware compares roles by value and
// an unrecognised one would simply fail every check.
func (u *User) ChangeRole(role UserRole) error {
	if !ValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}
	u.Role = role
	return nil
}

func (u *User) Activate() {
	u.IsActive = true
}

func (u *User) Deactivate() {
	u.IsActive = false
}

func (u *User) RecordLogin() {
	now := time.Now()
	u.LastLoginAt = &now
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsVendor() bool {
	return u.Role == RoleVendor
}

func (u *User) SetPhone(phone string) {
	u.Phone = phone
}
