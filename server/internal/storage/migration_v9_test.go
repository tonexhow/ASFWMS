package storage

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fixtureBeforeV9(ddl string) string {
	ddl = legacyV8DDL(ddl)
	ddl = strings.Replace(ddl, "INSERT INTO schema_migrations(number) VALUES(9),(10);", "", 1)
	ddl = strings.Replace(ddl, "CREATE UNIQUE INDEX faculty_id_number_unique ON faculty(faculty_id_number COLLATE NOCASE) WHERE faculty_id_number <> '';", "", 1)
	ddl = strings.Replace(ddl, "INSERT INTO schema_migrations(number) VALUES(9);", "", 1)
	for _, table := range []string{"subject_components", "subject_facility_requests", "generation_waits"} {
		ddl = regexp.MustCompile(`(?s)CREATE TABLE `+table+` \(.*?\n\);\n`).ReplaceAllString(ddl, "")
		ddl = regexp.MustCompile(`(?m)^CREATE (UNIQUE )?INDEX [^\n]* ON `+table+`[^\n]*\n`).ReplaceAllString(ddl, "")
		ddl = regexp.MustCompile(`(?m)^CREATE TRIGGER revision_`+table+`_[^\n]*\n`).ReplaceAllString(ddl, "")
	}
	return strings.Replace(ddl, "VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9);", "VALUES (1),(2),(3),(4),(5),(6),(7),(8);", 1)
}

func TestV9MigrationPreservesIDsNamesAndWorkingHours(t *testing.T) {
	raw, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	legacy := fixtureBeforeV9(string(raw))
	legacy = strings.Replace(legacy, "size INTEGER NOT NULL DEFAULT 50", "size INTEGER NOT NULL DEFAULT 45", 1)
	legacy = strings.Replace(legacy, "capacity INTEGER NOT NULL DEFAULT 50", "capacity INTEGER NOT NULL DEFAULT 45", 1)
	dir := t.TempDir()
	oldSchema := filepath.Join(dir, "v8.sql")
	os.WriteFile(oldSchema, []byte(legacy), 0600)
	path := filepath.Join(dir, "database.db")
	s, err := Open(path, oldSchema)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO departments(id,code,name) VALUES(1,'CITE','Technology');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'BSIT','IT');
 INSERT INTO subjects(id,department_id,code,title,units,weekly_minutes,required_room) VALUES(7,1,'LAB','Existing laboratory subject',3,180,'laboratory');
 INSERT INTO faculty(id,department_id,name) VALUES(8,1,'Dr. Existing Unsplit Name');
 INSERT INTO faculty_work_times(faculty_id,day,start_minute,end_minute) VALUES(8,1,540,1020);
 INSERT INTO faculty_preferences(faculty_id,first_start_minute,first_end_minute) VALUES(8,540,630);
 INSERT INTO sections(id,program_id,code,year_level,size) VALUES(9,1,'A',1,45),(10,1,'A',2,40);
 INSERT INTO buildings(id,code,name) VALUES(1,'B','Building');
 INSERT INTO rooms(id,building_id,code,name,capacity) VALUES(11,1,'L1','Lab',45),(12,1,'R2','Room',40);`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path, schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, test := range []struct {
		query string
		want  int
	}{
		{"SELECT max(number) FROM schema_migrations", 10}, {"SELECT size FROM sections WHERE id=9", 50}, {"SELECT size FROM sections WHERE id=10", 40},
		{"SELECT capacity FROM rooms WHERE id=11", 50}, {"SELECT capacity FROM rooms WHERE id=12", 40}, {"SELECT first_start_minute FROM faculty_preferences WHERE faculty_id=8", 0},
		{"SELECT start_minute FROM faculty_work_times WHERE faculty_id=8", 540}, {"SELECT count(*) FROM subject_components WHERE subject_id=7 AND kind='laboratory' AND units=3 AND weekly_minutes=180", 1},
	} {
		var n int
		if err := s.DB.QueryRow(test.query).Scan(&n); err != nil || n != test.want {
			t.Fatalf("%s: %d want %d / %v", test.query, n, test.want, err)
		}
	}
	var name, first string
	s.DB.QueryRow("SELECT name,first_name FROM faculty WHERE id=8").Scan(&name, &first)
	if name != "Dr. Existing Unsplit Name" || first != "" {
		t.Fatal("legacy name guessed or altered")
	}
	if _, err = s.DB.Exec(`INSERT INTO sections(program_id,code,year_level) VALUES(1,'B',1); INSERT INTO rooms(building_id,code,name) VALUES(1,'R3','Default room');`); err != nil {
		t.Fatal(err)
	}
	var size, capacity int
	s.DB.QueryRow("SELECT size FROM sections WHERE code='B'").Scan(&size)
	s.DB.QueryRow("SELECT capacity FROM rooms WHERE code='R3'").Scan(&capacity)
	if size != 50 || capacity != 50 {
		t.Fatalf("default sizes %d %d", size, capacity)
	}
	assertForeignKeyIntegrity(t, s.DB)
	backups, _ := os.ReadDir(filepath.Join(dir, "backups"))
	if len(backups) == 0 {
		t.Fatal("missing pre-migration backup")
	}
}
