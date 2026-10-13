package service

import (
	"context"
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
)

// UserService is the administration surface over user accounts: who exists,
// what role they hold, and whether they can sign in.
//
// Accounts are never deleted. A user carries loans, payments and audit entries,
// so removing the row would break the financial trail; deactivation is what
// revokes access.
type UserService struct {
	userRepo   port.UserRepository
	clientRepo port.ClientRepository
	vendorRepo port.VendorRepository
	audit      *AuditService
}

func NewUserService(
	userRepo port.UserRepository,
	clientRepo port.ClientRepository,
	vendorRepo port.VendorRepository,
	audit *AuditService,
) *UserService {
	return &UserService{userRepo: userRepo, clientRepo: clientRepo, vendorRepo: vendorRepo, audit: audit}
}

// hasProfile reports whether the account has the record its role needs.
//
// A client is a person with a national ID, a tax ID and an address; a vendor is
// a business. Those live in their own tables and are created by their own
// onboarding flows. An account holding one of those roles without the matching
// record can sign in but has nothing to show, which is how an orphan account
// gets made.
func (s *UserService) hasProfile(ctx context.Context, userID uuid.UUID, role model.UserRole) bool {
	switch role {
	case model.RoleClient:
		c, err := s.clientRepo.FindByUserID(ctx, userID)
		return err == nil && c != nil
	case model.RoleVendor:
		v, err := s.vendorRepo.FindByUserID(ctx, userID)
		return err == nil && v != nil
	default:
		// Administrators carry no separate profile.
		return true
	}
}

// List returns users matching the filter plus the total before pagination.
func (s *UserService) List(ctx context.Context, filter port.UserFilter) ([]model.User, int64, error) {
	if filter.Role != "" && !model.ValidRole(filter.Role) {
		return nil, 0, fmt.Errorf("invalid role: %s", filter.Role)
	}
	return s.userRepo.List(ctx, filter)
}

// Get returns one user.
func (s *UserService) Get(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return s.userRepo.FindByID(ctx, id)
}

// ChangeRole moves a user to another role.
//
// An administrator cannot change their own role: demoting yourself revokes the
// very permission needed to undo it, and if you are the last administrator it
// locks everyone out of the admin surface.
func (s *UserService) ChangeRole(ctx context.Context, actorID, targetID uuid.UUID, role model.UserRole) (*model.User, error) {
	if actorID == targetID {
		return nil, fmt.Errorf("you cannot change your own role")
	}

	user, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if !s.hasProfile(ctx, user.ID, role) {
		return nil, fmt.Errorf(
			"%s has no %s profile: create it through that role's own registration first",
			user.Email, role)
	}

	previous := user.Role
	if err := user.ChangeRole(role); err != nil {
		return nil, err
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.audit.Record(ctx, &actorID, "user_change_role", "user", user.ID.String(),
		fmt.Sprintf("Role of %s changed from %s to %s", user.Email, previous, role))
	return user, nil
}

// SetActive activates or deactivates a user.
//
// Deactivating yourself is refused for the same reason as a self demotion: it
// is an action you could not reverse afterwards.
func (s *UserService) SetActive(ctx context.Context, actorID, targetID uuid.UUID, active bool) (*model.User, error) {
	if actorID == targetID && !active {
		return nil, fmt.Errorf("you cannot deactivate your own account")
	}

	user, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if active {
		user.Activate()
	} else {
		user.Deactivate()
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	action, description := "user_deactivate", "deactivated"
	if active {
		action, description = "user_activate", "activated"
	}
	s.audit.Record(ctx, &actorID, action, "user", user.ID.String(),
		fmt.Sprintf("Account %s %s", user.Email, description))
	return user, nil
}

// Create registers a staff account: an administrator or an advisor.
//
// Those are the roles that are just an account. Clients and vendors are a
// person or a business with an identity document and an address, and their own
// onboarding builds both records together; creating the account alone leaves
// one that can sign in and has nothing to show.
func (s *UserService) Create(ctx context.Context, actorID uuid.UUID, email, phone string, role model.UserRole) (*model.User, error) {
	if role != model.RoleAdmin && role != model.RoleAdvisor {
		return nil, fmt.Errorf(
			"only staff accounts can be created here: register a %s through its own flow, which also creates the profile",
			role)
	}

	if existing, err := s.userRepo.FindByEmail(ctx, email); err == nil && existing != nil {
		return nil, fmt.Errorf("a user with that email already exists")
	}
	// A removed account keeps its row, and that row still holds the unique
	// email index, so inserting here would fail on a constraint the operator
	// cannot see. Say what actually happened instead.
	if removed, err := s.userRepo.FindRemovedByEmail(ctx, email); err == nil && removed != nil {
		return nil, fmt.Errorf(
			"%s belonged to an account that was removed; it has to be restored rather than created again", email)
	}

	// Credentials live outside this table: sign-in is a one-time code sent to
	// the address, or a verified Firebase identity. The UID is a placeholder
	// that satisfies the unique, non-null column until one is bound.
	user, err := model.NewUser(uuid.New().String(), email, role)
	if err != nil {
		return nil, err
	}
	user.SetPhone(phone)

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.audit.Record(ctx, &actorID, "user_create", "user", user.ID.String(),
		fmt.Sprintf("User %s created with role %s", email, role))
	return user, nil
}
