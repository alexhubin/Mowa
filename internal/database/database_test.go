package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrationsAndEnablesWAL(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "mova.db") + "?_pragma=busy_timeout%285000%29&_pragma=foreign_keys%281%29"
	db, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('users', 'sessions', 'rooms')").Scan(&count); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if count != 3 {
		t.Fatalf("migrated tables = %d, want 3", count)
	}
}
