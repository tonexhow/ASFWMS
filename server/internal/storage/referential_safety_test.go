package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSafeRelationsKeepIndependentRows(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	s, err := Open(filepath.Join(dir, "live.db"), filepath.Join(root, "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.DB.Exec(`
 INSERT INTO departments(id,code,name) VALUES(1,'TEST','Test');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'P','Program');
 INSERT INTO subjects(id,department_id,code,title) VALUES(1,1,'CP1','Programming');
 INSERT INTO curricula(id,program_id,name) VALUES(1,1,'First curriculum');
 INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1);
 INSERT INTO academic_periods(id,label,academic_year,semester) VALUES(1,'Year','2026-2027','First');
 INSERT INTO faculty(id,department_id,name) VALUES(1,1,'Instructor');
 INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level,adviser_id) VALUES(1,1,1,1,'P 1A',1,1);
 INSERT INTO faculty_subjects(id,faculty_id,subject_id) VALUES(1,1,1);
 INSERT INTO teaching_assignments(id,faculty_id,subject_id,section_id) VALUES(1,1,1,1);
 INSERT INTO buildings(id,department_id,code,name) VALUES(1,1,'BLDG','Building');
 INSERT INTO rooms(id,building_id,department_id,code,name) VALUES(1,1,1,'R1','Room');
 `)
	if err != nil {
		t.Fatal(err)
	}
	// Raw DELETE is intentionally blocked until junction rows are explicitly unlinked.
	if _, err = s.DB.Exec("DELETE FROM curricula WHERE id=1"); err == nil {
		t.Fatal("curriculum deleted despite existing memberships")
	}
	if _, err = s.DB.Exec("DELETE FROM curriculum_subject_years WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM curricula WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	var curriculum sql.NullInt64
	if err = s.DB.QueryRow("SELECT curriculum_id FROM sections WHERE id=1").Scan(&curriculum); err != nil || curriculum.Valid {
		t.Fatalf("section should become unassigned: %v %v", curriculum, err)
	}
	var count int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM subjects").Scan(&count); err != nil || count != 1 {
		t.Fatal("unrelated subject deleted")
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM sections").Scan(&count); err != nil || count != 1 {
		t.Fatal("section deleted")
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM teaching_assignments").Scan(&count); err != nil || count != 1 {
		t.Fatal("teaching assignment deleted")
	}
	if _, err = s.DB.Exec("DELETE FROM buildings WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	var building sql.NullInt64
	if err = s.DB.QueryRow("SELECT building_id FROM rooms WHERE id=1").Scan(&building); err != nil || building.Valid {
		t.Fatalf("room should become building-unassigned: %v %v", building, err)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil || count != 1 {
		t.Fatal("room deleted")
	}
	if _, err = s.DB.Exec("DELETE FROM academic_periods WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	var period sql.NullInt64
	if err = s.DB.QueryRow("SELECT period_id FROM sections WHERE id=1").Scan(&period); err != nil || period.Valid {
		t.Fatalf("period reference not cleared: %v %v", period, err)
	}
	if _, err = s.DB.Exec("DELETE FROM faculty_subjects WHERE id=1"); err == nil {
		t.Fatal("deleted eligibility still referenced by instructor assignment")
	}
	assertForeignKeyIntegrity(t, s.DB)
}

func TestSavedScheduleMustBeReleasedBeforeLinksChange(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	s, err := Open(filepath.Join(t.TempDir(), "with-schedule.db"), filepath.Join(root, "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.DB.Exec(`
 INSERT INTO departments(id,code,name) VALUES(1,'TEST','Test');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'P','Program');
 INSERT INTO subjects(id,department_id,code,title) VALUES(1,1,'CP1','Programming');
 INSERT INTO curricula(id,program_id,name) VALUES(1,1,'First curriculum');
 INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1);
 INSERT INTO academic_periods(id,label,academic_year,semester) VALUES(1,'Year','2026-2027','First');
 INSERT INTO faculty(id,department_id,name) VALUES(1,1,'Instructor');
 INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level) VALUES(1,1,1,1,'P 1A',1);
 INSERT INTO faculty_subjects(id,faculty_id,subject_id) VALUES(1,1,1);
 INSERT INTO teaching_assignments(id,faculty_id,subject_id,section_id) VALUES(1,1,1,1);
 INSERT INTO buildings(id,department_id,code,name) VALUES(1,1,'BLDG','Building');
 INSERT INTO rooms(id,building_id,department_id,code,name) VALUES(1,1,1,'R1','Room');
 INSERT INTO generation_runs(id,department_id,period_id,mode,seed,revision,status,outcome,result_json) VALUES(1,1,1,'standard',42,0,'committed','complete','{}');
 INSERT INTO scheduled_meetings(id,run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed)
 VALUES(1,1,1,1,1,1,1,1,1,480,540,0);
 `)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"UPDATE sections SET curriculum_id=NULL WHERE id=1",
		"UPDATE sections SET period_id=NULL WHERE id=1",
		"DELETE FROM curriculum_subject_years WHERE id=1",
		"UPDATE curriculum_subject_years SET year_level=2 WHERE id=1",
		"DELETE FROM faculty_subjects WHERE id=1",
		"DELETE FROM teaching_assignments WHERE id=1",
	} {
		if _, err = s.DB.Exec(query); err == nil {
			t.Fatalf("SQL changed a committed timetable without release: %s", query)
		}
	}
	// Once meetings and the associated run are released, the links can be removed.
	if _, err = s.DB.Exec("DELETE FROM scheduled_meetings WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM generation_runs WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM teaching_assignments WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM faculty_subjects WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM curriculum_subject_years WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DELETE FROM curricula WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	assertForeignKeyIntegrity(t, s.DB)
}

func TestMigrationFourToFivePreservesRowsAndTriggers(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	schemaPath := filepath.Join(root, "database", "schema.sql")
	text, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a release-4 SQLite schema without its new link actions or guards.
	legacy := fixtureBeforeV9(string(text))
	// v4 had no faculty preferences table or its revision triggers.
	legacy = regexp.MustCompile(`(?s)CREATE TABLE faculty_preferences \(.*?\n\);\n`).ReplaceAllString(legacy, "")
	legacy = regexp.MustCompile(`(?m)^CREATE TRIGGER revision_faculty_preferences_[^\n]+\n`).ReplaceAllString(legacy, "")
	legacy = strings.Replace(legacy, "INSERT INTO schema_migrations(number) VALUES (1),(2),(3),(4),(5),(6),(7),(8);", "INSERT INTO schema_migrations(number) VALUES (1),(2),(3),(4);", 1)
	legacy = curriculumDDLBeforeScope(legacy)
	legacy = strings.ReplaceAll(legacy, "ON DELETE SET NULL", "ON DELETE RESTRICT")
	legacy = regexp.MustCompile(`(?m)^CREATE (UNIQUE )?INDEX idx_[^;]+;\n`).ReplaceAllString(legacy, "")
	for _, name := range []string{"guard_section_context_update", "guard_membership_delete", "guard_membership_update", "guard_faculty_eligibility_delete", "guard_instructor_assignment_delete"} {
		re := regexp.MustCompile(`(?s)CREATE TRIGGER ` + name + ` .*?END;\n`)
		legacy = re.ReplaceAllString(legacy, "")
	}
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO departments(id,code,name) VALUES(1,'CITE','Department');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'BSIT','Degree');
 INSERT INTO curricula(id,program_id,name) VALUES(1,1,'Curriculum');
 INSERT INTO subjects(id,department_id,code,title) VALUES(1,1,'CP1','Programming');
 INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1);
 INSERT INTO academic_periods(id,label,academic_year,semester) VALUES(1,'Test','2026-2027','First');
 INSERT INTO faculty(id,department_id,name) VALUES(1,1,'Actual name');
 INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level) VALUES(1,1,1,1,'1A',1);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path, schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var v, subjects, sections int
	if err = upgraded.DB.QueryRow("SELECT max(number) FROM schema_migrations").Scan(&v); err != nil || v != 10 {
		t.Fatalf("expected v9, got %d %v", v, err)
	}
	_ = upgraded.DB.QueryRow("SELECT COUNT(*) FROM subjects").Scan(&subjects)
	_ = upgraded.DB.QueryRow("SELECT COUNT(*) FROM sections").Scan(&sections)
	if subjects != 1 || sections != 1 {
		t.Fatal("schema upgrade dropped business data")
	}
	if _, err = upgraded.DB.Exec("DELETE FROM curriculum_subject_years WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.DB.Exec("DELETE FROM curricula WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	var cv sql.NullInt64
	if err = upgraded.DB.QueryRow("SELECT curriculum_id FROM sections WHERE id=1").Scan(&cv); err != nil || cv.Valid {
		t.Fatalf("upgraded FK failed: %v %v", cv, err)
	}
	var backups []os.DirEntry
	backups, err = os.ReadDir(filepath.Join(filepath.Dir(path), "backups"))
	if err != nil || len(backups) == 0 {
		t.Fatal("migration did not create safety backup")
	}
	assertForeignKeyIntegrity(t, upgraded.DB)
}

func assertForeignKeyIntegrity(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	rs, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	for rs.Next() {
		count++
	}
	if err = rs.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("foreign key violations: %d", count)
	}
}
