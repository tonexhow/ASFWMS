package storage

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMigrationFiveToSixPreservesIndependentRows(t *testing.T) {
	schema := schemaPath(t)
	raw, err := os.ReadFile(schema)
	if err != nil {
		t.Fatal(err)
	}
	legacy := curriculumDDLBeforeScope(fixtureBeforeV9(string(raw)))
	// A v5 fixture predates preferred facility/time windows and the v8 bank index.
	legacy = regexp.MustCompile(`(?s)CREATE TABLE faculty_preferences \(.*?\n\);\n`).ReplaceAllString(legacy, "")
	legacy = regexp.MustCompile(`(?m)^CREATE TRIGGER revision_faculty_preferences_[^\n]+\n`).ReplaceAllString(legacy, "")
	legacy = regexp.MustCompile(`(?s)CREATE UNIQUE INDEX idx_sections_bank_unique ON sections\(.*?;\n`).ReplaceAllString(legacy, "")
	legacy = strings.Replace(legacy, "INSERT INTO schema_migrations(number) VALUES (1),(2),(3),(4),(5),(6),(7),(8);", "INSERT INTO schema_migrations(number) VALUES (1),(2),(3),(4),(5);", 1)
	legacy = strings.Replace(legacy, `CREATE UNIQUE INDEX idx_curricula_scoped_unique ON curricula(program_id,year_level,semester)
 WHERE active=1 AND program_id IS NOT NULL AND year_level>0;`, "", 1)
	path := filepath.Join(t.TempDir(), "database.db")
	// This test may create a v5 fixture using the schema we expect to migrate.
	// It doesn't touch the user's shipped database.
	temp, err := os.CreateTemp(filepath.Dir(path), "v5-fixture-*.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = temp.WriteString(legacy)
	temp.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(temp.Name())
	db, err := Open(path, temp.Name())
	if err != nil {
		t.Fatal(err)
	}
	// Open(new) assumes schema is current and therefore creates v5 as supplied.
	_, err = db.DB.Exec(`INSERT INTO departments(id,code,name) VALUES(1,'CITE','Technology');
        INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'BSIT','Technology program');
        INSERT INTO curricula(id,program_id,name) VALUES(1,1,'Multi-year curriculum');
        INSERT INTO subjects(id,department_id,code,title) VALUES(1,1,'CP1','Programming');
        INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	upgraded, err := Open(path, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var v, year, subjects, links int
	var semester string
	if err = upgraded.DB.QueryRow("SELECT max(number) FROM schema_migrations").Scan(&v); err != nil || v != 9 {
		t.Fatalf("migration version %d %v", v, err)
	}
	if err = upgraded.DB.QueryRow("SELECT year_level,semester FROM curricula WHERE id=1").Scan(&year, &semester); err != nil || year != 0 || semester != "" {
		t.Fatalf("legacy scope changed %d %q %v", year, semester, err)
	}
	upgraded.DB.QueryRow("SELECT COUNT(*) FROM subjects").Scan(&subjects)
	upgraded.DB.QueryRow("SELECT COUNT(*) FROM curriculum_subject_years").Scan(&links)
	if subjects != 1 || links != 1 {
		t.Fatal("migration changed subjects or membership rows")
	}
	if _, err = upgraded.DB.Exec("INSERT INTO curricula(program_id,name,year_level,semester) VALUES(1,'First semester',1,'1st Semester')"); err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.DB.Exec("INSERT INTO curricula(program_id,name,year_level,semester) VALUES(1,'Duplicate first semester',1,'1st Semester')"); err == nil {
		t.Fatal("unique scope index missing")
	}
	if _, err = upgraded.DB.Exec("INSERT INTO curricula(program_id,name,year_level,semester) VALUES(1,'Second semester',1,'2nd Semester')"); err != nil {
		t.Fatal(err)
	}
	bad, err := ReadRows(context.Background(), upgraded.DB, "PRAGMA foreign_key_check")
	if err != nil || len(bad) != 0 {
		t.Fatalf("invalid foreign keys after migration: %v %v", bad, err)
	}
	if backup, err := os.ReadDir(filepath.Join(filepath.Dir(path), "backups")); err != nil || len(backup) == 0 {
		t.Fatalf("no pre-migration backup created: %v", err)
	}
}
