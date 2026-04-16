package service

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

func TestCreateAPIKeyRejectsInvalidCustomKey(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-invalid")
	svc := NewAmpService()

	_, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name:      "invalid",
		CustomKey: "bad-key",
	})
	if !errors.Is(err, ErrInvalidAPIKeyFormat) {
		t.Fatalf("expected ErrInvalidAPIKeyFormat, got %v", err)
	}
}

func TestCreateAPIKeyRejectsDuplicateCustomKey(t *testing.T) {
	setupAmpServiceTestDB(t)

	firstUser := createAmpServiceTestUser(t, "user-first")
	secondUser := createAmpServiceTestUser(t, "user-second")
	svc := NewAmpService()

	req := &model.CreateAPIKeyRequest{
		Name:      "custom",
		CustomKey: "Key1234567890ABCD",
	}

	if _, err := svc.CreateAPIKey(firstUser.ID, req); err != nil {
		t.Fatalf("first CreateAPIKey returned error: %v", err)
	}

	_, err := svc.CreateAPIKey(secondUser.ID, req)
	if !errors.Is(err, ErrDuplicateAPIKey) {
		t.Fatalf("expected ErrDuplicateAPIKey, got %v", err)
	}
}

func TestUpdateAPIKeyUpdatesNameAndExpiry(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-update-key")
	svc := NewAmpService()

	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name: "before",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	updated, err := svc.UpdateAPIKey(user.ID, created.ID, &model.UpdateAPIKeyRequest{
		Name:      "after",
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey returned error: %v", err)
	}

	if updated.Name != "after" {
		t.Fatalf("expected updated name %q, got %q", "after", updated.Name)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt %v, got %v", expiresAt, updated.ExpiresAt)
	}
}

func setupAmpServiceTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "amp-service-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}

	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func createAmpServiceTestUser(t *testing.T, username string) *model.User {
	t.Helper()

	user := &model.User{
		Username:      username,
		PasswordHash:  "hash",
		BalanceMicros: 0,
	}

	if err := repository.NewUserRepository().Create(user); err != nil {
		t.Fatalf("create user returned error: %v", err)
	}

	return user
}
