package database

import (
	"context"
	"os"
	"testing"
)

func TestOpenRunsPostgresMigrations(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for the PostgreSQL integration test")
	}
	db, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if version < 180000 || version >= 190000 {
		t.Fatalf("PostgreSQL version number = %d, want 18.x", version)
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('users', 'user_settings', 'sessions', 'rooms', 'room_members', 'friend_requests', 'friendships', 'direct_calls', 'passkey_users', 'passkeys', 'passkey_ceremonies')").Scan(&count); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if count != 11 {
		t.Fatalf("migrated tables = %d, want 11", count)
	}
}
