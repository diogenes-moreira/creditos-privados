package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) FindByFirebaseUID(ctx context.Context, uid string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "firebase_uid = ?", uid).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "phone = ?", phone).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// List returns users matching the filter, newest first, with the total count
// before pagination so the UI can page through them.
func (r *UserRepository) List(ctx context.Context, filter port.UserFilter) ([]model.User, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.User{})

	if filter.Role != "" {
		q = q.Where("role = ?", filter.Role)
	}
	if filter.Active != nil {
		q = q.Where("is_active = ?", *filter.Active)
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		q = q.Where("LOWER(email) LIKE ? OR LOWER(phone) LIKE ?", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var users []model.User
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	return users, total, nil
}

// FindRemovedByEmail finds a soft-deleted account by email.
func (r *UserRepository) FindRemovedByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Unscoped().
		Where("email = ? AND deleted_at IS NOT NULL", email).
		First(&user).Error; err != nil {
		return nil, fmt.Errorf("no removed user with that email: %w", err)
	}
	return &user, nil
}
