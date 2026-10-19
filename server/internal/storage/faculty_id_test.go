package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFacultyIDMigrationPreservesRecordsAndCreatesBackup(t *testing.T) {
	// A true v9 database, with existing faculty and associated rows.
	root := t.TempDir()
	raw, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(raw)
	legacy = removeFacultyIDForV9(legacy)
	schemaFile := filepath.Join(root, "v9.sql")
	if err := os.WriteFile(schemaFile, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "database.db")
	store, err := Open(path, schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`INSERT INTO departments(id,code,name) VALUES(1,'CITE','Technology');
        INSERT INTO faculty(id,department_id,name) VALUES(7,1,'Legacy Faculty');
        INSERT INTO faculty_work_times(faculty_id,day,start_minute,end_minute) VALUES(7,1,480,600);`)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	upgraded, err := Open(path, schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var v, n, start int
	var name, identifier string
	if err := upgraded.DB.QueryRow("SELECT max(number) FROM schema_migrations").Scan(&v); err != nil || v != 10 {
		t.Fatalf("version %d %v", v, err)
	}
	if err := upgraded.DB.QueryRow("SELECT name,faculty_id_number FROM faculty WHERE id=7").Scan(&name, &identifier); err != nil || name != "Legacy Faculty" || identifier != "" {
		t.Fatalf("faculty altered %q %q %v", name, identifier, err)
	}
	upgraded.DB.QueryRow("SELECT start_minute FROM faculty_work_times WHERE faculty_id=7").Scan(&start)
	if start != 480 {
		t.Fatal("working hours not preserved")
	}
	if _, err = upgraded.DB.Exec(`INSERT INTO faculty(name,faculty_id_number) VALUES('Another','EMP-12'); INSERT INTO faculty(name,faculty_id_number) VALUES('Anonymous','');`); err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.DB.Exec(`INSERT INTO faculty(name,faculty_id_number) VALUES('Duplicate','emp-12')`); err == nil {
		t.Fatal("duplicate Faculty ID permitted")
	}
	upgraded.DB.QueryRow("SELECT count(*) FROM faculty").Scan(&n)
	if n != 3 {
		t.Fatalf("unexpected number of faculty: %d", n)
	}
	assertForeignKeyIntegrity(t, upgraded.DB)
	backups, _ := os.ReadDir(filepath.Join(root, "backups"))
	if len(backups) == 0 {
		t.Fatal("migration did not create a backup")
	}
}

func removeFacultyIDForV9(schema string) string {
	// Mirrors the exact v9 table shape and index absence, but retains every
	// other table and constraint from the checked-in schema.
	schema = replaceOnce(schema, "INSERT INTO schema_migrations(number) VALUES(9),(10);", "INSERT INTO schema_migrations(number) VALUES(9);")
	schema = replaceOnce(schema, "faculty_id_number TEXT NOT NULL DEFAULT '', ", "")
	schema = replaceOnce(schema, "CREATE UNIQUE INDEX faculty_id_number_unique ON faculty(faculty_id_number COLLATE NOCASE) WHERE faculty_id_number <> '';", "")
	return schema
}
func replaceOnce(input, old, updated string) string {
	return strings.Replace(input, old, updated, 1)
}
