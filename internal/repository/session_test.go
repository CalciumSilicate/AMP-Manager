package repository

import (
	"testing"
	"time"

	"ampmanager/internal/database"
)

func TestListFiltersBySessionIDContains(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	_, err := db.Exec(`
		INSERT INTO request_logs (
			id, created_at, user_id, api_key_id, original_model, session_id, method, path, status_code, latency_ms, is_streaming
		) VALUES
			('log-session-a', ?, 'user-1', 'key-user-1', 'gpt-5.4', 'sess-alpha-1', 'POST', '/v1/responses', 200, 100, 0),
			('log-session-b', ?, 'user-1', 'key-user-1', 'gpt-5.4', 'sess-beta-1', 'POST', '/v1/responses', 200, 100, 0)
	`, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert request_logs returned error: %v", err)
	}

	repo := NewRequestLogRepository()
	items, total, err := repo.List(ListParams{
		SessionID: "alpha",
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one matching session log, total=%d len=%d", total, len(items))
	}
	if items[0].SessionID == nil || *items[0].SessionID != "sess-alpha-1" {
		t.Fatalf("unexpected session id: %+v", items[0].SessionID)
	}
}

func TestGetSessionLeaderboardCountsDistinctSessionsPerUser(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	_, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, is_admin, created_at) VALUES
			('user-1', 'alpha', 'hash', 0, ?),
			('user-2', 'beta', 'hash', 0, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert users returned error: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO request_logs (
			id, created_at, user_id, api_key_id, original_model, session_id, method, path, status_code, latency_ms, is_streaming
		) VALUES
			('leader-1', ?, 'user-1', 'key-user-1', 'gpt-5.4', 'sess-a', 'POST', '/v1/responses', 200, 100, 0),
			('leader-2', ?, 'user-1', 'key-user-1', 'gpt-5.4', 'sess-b', 'POST', '/v1/responses', 200, 100, 0),
			('leader-3', ?, 'user-1', 'key-user-1', 'gpt-5.4', 'sess-b', 'POST', '/v1/responses', 200, 100, 0),
			('leader-4', ?, 'user-2', 'key-user-2', 'gpt-5.4', 'sess-c', 'POST', '/v1/responses', 200, 100, 0)
	`, now, now.Add(time.Second), now.Add(2*time.Second), now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("insert request_logs returned error: %v", err)
	}

	repo := NewRequestLogRepository()
	items, err := repo.GetSessionLeaderboard(5*time.Minute, 10)
	if err != nil {
		t.Fatalf("GetSessionLeaderboard returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 leaderboard rows, got %d", len(items))
	}
	if items[0].Username != "alpha" || items[0].DistinctSessionCount != 2 {
		t.Fatalf("unexpected first leaderboard row: %+v", items[0])
	}
	if items[1].Username != "beta" || items[1].DistinctSessionCount != 1 {
		t.Fatalf("unexpected second leaderboard row: %+v", items[1])
	}
}
