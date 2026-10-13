// Command usertool performs the account operations the product deliberately
// does not expose over HTTP.
//
// Seeding is all-or-nothing: cmd/seed also inserts fifty synthetic clients and
// their portfolios, which does not belong in a real deployment. This acts on
// exactly one account.
//
// ACTION=promote (default) creates or promotes one email to a staff role,
// administrator unless ADMIN_ROLE says advisor. It is idempotent: an existing
// user is updated and reactivated, not duplicated.
//
// ACTION=remove soft-deletes one account. There is no delete endpoint and there
// should not be: a user carries loans, payments and audit entries, so removing
// the row would break the financial trail. This is for operational cleanup of
// records created in error, and it is a soft delete precisely so the audit
// entries that reference the user still resolve.
//
//	ADMIN_EMAIL=someone@example.com ACTION=remove go run ./cmd/usertool
package main

import (
	"log"
	"os"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/config"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/persistence/postgres"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	email := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	if email == "" {
		log.Fatal("ADMIN_EMAIL is required")
	}
	phone := strings.TrimSpace(os.Getenv("ADMIN_PHONE"))
	role := model.UserRole(strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_ROLE"))))
	if role == "" {
		role = model.RoleAdmin
	}
	// Only staff roles: a client or vendor needs the profile its own onboarding
	// builds, and this tool creates the account alone.
	if role != model.RoleAdmin && role != model.RoleAdvisor {
		log.Fatalf("ADMIN_ROLE must be admin or advisor, got %q", role)
	}

	action := strings.ToLower(strings.TrimSpace(os.Getenv("ACTION")))
	if action == "" {
		action = "promote"
	}
	if action != "promote" && action != "remove" {
		log.Fatalf("ACTION must be promote or remove, got %q", action)
	}

	cfg := config.Load()
	db, err := postgres.NewConnection(cfg.DB)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// The schema is owned by the server's own migration; only make sure the
	// users table is present so this can run against a fresh database.
	if err := postgres.AutoMigrate(db, &model.User{}); err != nil {
		log.Fatalf("Failed to migrate users: %v", err)
	}

	var existing model.User
	found := db.Where("email = ?", email).First(&existing).Error == nil

	if action == "remove" {
		if !found {
			log.Fatalf("No user with email %s", email)
		}
		// Soft delete: GORM sets deleted_at, so listings skip it while the
		// audit entries that reference this id still resolve.
		if err := db.Delete(&existing).Error; err != nil {
			log.Fatalf("Failed to remove %s: %v", email, err)
		}
		log.Printf("Account removed: %s (ID: %s, role was %s)", email, existing.ID, existing.Role)
		return
	}

	if found {
		existing.Role = role
		existing.IsActive = true
		if phone != "" {
			existing.Phone = phone
		}
		if err := db.Save(&existing).Error; err != nil {
			log.Fatalf("Failed to promote %s: %v", email, err)
		}
		log.Printf("Account updated: %s (ID: %s, role: %s)", email, existing.ID, role)
		return
	}

	user := &model.User{
		ID: uuid.New(),
		// Firebase owns credentials in this system. With Firebase sign-in not
		// configured there is no external identity to bind to, so this is a
		// placeholder that keeps the unique, non-null column satisfied.
		FirebaseUID: uuid.New().String(),
		Email:       email,
		Phone:       phone,
		Role:        role,
		IsActive:    true,
	}
	if err := db.Create(user).Error; err != nil {
		log.Fatalf("Failed to create %s: %v", email, err)
	}
	log.Printf("Account created: %s (ID: %s, role: %s)", email, user.ID, role)
}
