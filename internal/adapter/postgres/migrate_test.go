package postgres

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsOrdersVersionsAndChecksContent(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000002_second.sql": {Data: []byte("SELECT 2;")},
		"migrations/000001_first.sql":  {Data: []byte("SELECT 1;")},
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].version != 1 || migrations[1].version != 2 {
		t.Fatalf("unexpected migration order: %+v", migrations)
	}
	if migrations[0].checksum == migrations[1].checksum {
		t.Fatal("different migration bodies have the same checksum")
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000001_first.sql":  {Data: []byte("SELECT 1;")},
		"migrations/000001_second.sql": {Data: []byte("SELECT 2;")},
	}
	_, err := loadMigrations(files)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate version error, got %v", err)
	}
}
