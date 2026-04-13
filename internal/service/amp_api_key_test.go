package service

import (
	"errors"
	"path/filepath"
	"testing"

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
