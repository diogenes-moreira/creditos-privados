// Command createadmin grants one email administrator access.
//
// It exists because seeding is all-or-nothing: cmd/seed also inserts fifty
// synthetic clients and their portfolios, which does not belong in a real
// deployment. This creates or promotes exactly one account and nothing else.
//
// It is idempotent: an existing user is promoted to admin and reactivated
// rather than duplicated.
//
//	ADMIN_EMAIL=someone@example.com ADMIN_PHONE=+57... go run ./cmd/createadmin
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
	if err := db.Where("email = ?", email).First(&existing).Error; err == nil {
		existing.Role = model.RoleAdmin
		existing.IsActive = true
		if phone != "" {
			existing.Phone = phone
		}
		if err := db.Save(&existing).Error; err != nil {
			log.Fatalf("Failed to promote %s: %v", email, err)
		}
		log.Printf("Admin promoted: %s (ID: %s)", email, existing.ID)
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
		Role:        model.RoleAdmin,
		IsActive:    true,
	}
	if err := db.Create(user).Error; err != nil {
		log.Fatalf("Failed to create %s: %v", email, err)
	}
	log.Printf("Admin created: %s (ID: %s)", email, user.ID)
}
