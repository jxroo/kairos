package memory

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens a fresh SQLite database in a temp directory.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// tableExists reports whether a table with the given name exists.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&count)
	if err != nil {
		t.Fatalf("checking table %q: %v", name, err)
	}
	return count > 0
}

// TestMigrate_FreshDB verifies that Migrate creates all expected tables.
func TestMigrate_FreshDB(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate returned unexpected error: %v", err)
	}

	tables := []string{
		"schema_version",
		"memories",
		"memory_tags",
		"entities",
		"memory_entities",
		"documents",
		"chunks",
	}
	for _, tbl := range tables {
		if !tableExists(t, db, tbl) {
			t.Errorf("expected table %q to exist after migration", tbl)
		}
	}
}

// TestMigrate_Idempotent verifies that calling Migrate twice returns no error.
func TestMigrate_Idempotent(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate (idempotent): %v", err)
	}
}

// TestMigrate_VersionTracking verifies that schema_version records the latest version.
func TestMigrate_VersionTracking(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("querying schema_version: %v", err)
	}
	wantVersion := len(migrations)
	if version != wantVersion {
		t.Errorf("expected schema version %d, got %d", wantVersion, version)
	}
}

// TestMigrate_FKCascade verifies that deleting a memory cascades to
// memory_tags and memory_entities.
func TestMigrate_FKCascade(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Enable FK enforcement for this connection.
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("enabling foreign_keys: %v", err)
	}

	// Insert a memory, a tag, an entity, and a memory_entity link.
	if _, err := db.Exec(
		`INSERT INTO memories (id, content) VALUES ('m1', 'test content')`,
	); err != nil {
		t.Fatalf("inserting memory: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO memory_tags (memory_id, tag) VALUES ('m1', 'go')`,
	); err != nil {
		t.Fatalf("inserting memory_tag: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO entities (id, name, entity_type) VALUES ('e1', 'Go', 'language')`,
	); err != nil {
		t.Fatalf("inserting entity: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO memory_entities (memory_id, entity_id) VALUES ('m1', 'e1')`,
	); err != nil {
		t.Fatalf("inserting memory_entity: %v", err)
	}

	// Delete the memory — cascade should remove the child rows.
	if _, err := db.Exec(`DELETE FROM memories WHERE id='m1'`); err != nil {
		t.Fatalf("deleting memory: %v", err)
	}

	tables := []string{"memory_tags", "memory_entities"}
	for _, tbl := range tables {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+tbl).Scan(&count); err != nil {
			t.Fatalf("counting %s: %v", tbl, err)
		}
		if count != 0 {
			t.Errorf("expected %s to be empty after cascade delete, got %d rows", tbl, count)
		}
	}
}

// TestMigrate_CheckConstraint verifies that the importance CHECK constraint
// rejects values not in ('low','normal','high').
func TestMigrate_CheckConstraint(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tests := []struct {
		importance string
		wantErr    bool
	}{
		{"low", false},
		{"normal", false},
		{"high", false},
		{"critical", true},
		{"", true},
		{"NORMAL", true},
	}

	for i, tc := range tests {
		name := tc.importance
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			id := fmt.Sprintf("mem-%d", i)
			_, err := db.Exec(
				`INSERT INTO memories (id, content, importance) VALUES (?, 'c', ?)`,
				id, tc.importance,
			)
			if tc.wantErr && err == nil {
				t.Errorf("importance=%q: expected error, got nil", tc.importance)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("importance=%q: unexpected error: %v", tc.importance, err)
			}
		})
	}
}
