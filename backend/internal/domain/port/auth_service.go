package port

import "context"

type AuthUser struct {
	UID   string
	Email string
}

type AuthService interface {
	CreateUser(ctx context.Context, email, password string) (*AuthUser, error)
	VerifyToken(ctx context.Context, token string) (*AuthUser, error)
	DeleteUser(ctx context.Context, uid string) error
}
