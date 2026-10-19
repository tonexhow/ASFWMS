package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// A frozen pre-update schema exercises a real upgrade rather than a fresh install.
const legacySchemaForTest = `-- ASFWMS local schema. Numbered schema state lives in schema_migrations.
-- This file creates a NEW database. Startup never overwrites an existing one.
PRAGMA foreign_keys = ON;
CREATE TABLE schema_migrations (number INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
INSERT INTO schema_migrations(number) VALUES (1);
CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO app_meta VALUES ('revision','0'),('dataset','empty');
CREATE TABLE departments (
 id INTEGER PRIMARY KEY, code TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK(length(trim(code))>0),
 name TEXT NOT NULL CHECK(length(trim(name))>0), active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE programs (
 id INTEGER PRIMARY KEY, department_id INTEGER NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE academic_periods (
 id INTEGER PRIMARY KEY, label TEXT NOT NULL UNIQUE, academic_year TEXT NOT NULL, semester TEXT NOT NULL, term TEXT NOT NULL DEFAULT '',
 start_date TEXT NOT NULL, end_date TEXT NOT NULL CHECK(end_date>=start_date), active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)),
 allow_saturday INTEGER NOT NULL DEFAULT 0 CHECK(allow_saturday IN(0,1)),
 day_start INTEGER NOT NULL DEFAULT 420 CHECK(day_start>=0 AND day_start<1440),
 day_end INTEGER NOT NULL DEFAULT 1140 CHECK(day_end>day_start AND day_end<=1440),
 lunch_start INTEGER NOT NULL DEFAULT 660 CHECK(lunch_start>=0 AND lunch_start<1440),
 lunch_end INTEGER NOT NULL DEFAULT 840 CHECK(lunch_end>lunch_start AND lunch_end<=1440),
 lunch_minutes INTEGER NOT NULL DEFAULT 0 CHECK(lunch_minutes>=0 AND lunch_minutes<=lunch_end-lunch_start),
 max_consecutive INTEGER NOT NULL DEFAULT 0 CHECK(max_consecutive BETWEEN 0 AND 10)
);
CREATE TABLE subjects (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE RESTRICT,
 code TEXT NOT NULL COLLATE NOCASE, title TEXT NOT NULL, units REAL NOT NULL DEFAULT 3 CHECK(units>=0),
 weekly_minutes INTEGER NOT NULL DEFAULT 180 CHECK(weekly_minutes>0 AND weekly_minutes<=3600),
 priority TEXT NOT NULL DEFAULT 'major' CHECK(priority IN('major','minor')),
 required_room TEXT NOT NULL DEFAULT 'any' CHECK(required_room IN('any','classroom','laboratory','field')),
 pattern TEXT NOT NULL DEFAULT 'auto' CHECK(pattern IN('auto','mwf','tth','mw','mf','wf','sat','single','flex')),
 allowed_days TEXT NOT NULL DEFAULT 'weekdays' CHECK(allowed_days IN('weekdays','saturday','weekdays_saturday')),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(department_id,code)
);
CREATE TABLE curricula (
 id INTEGER PRIMARY KEY, program_id INTEGER NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
 name TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(program_id,name)
);
CREATE TABLE curriculum_subject_years (
 id INTEGER PRIMARY KEY, curriculum_id INTEGER NOT NULL REFERENCES curricula(id) ON DELETE RESTRICT,
 subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
 year_level INTEGER NOT NULL CHECK(year_level BETWEEN 1 AND 6),
 source_semester TEXT NOT NULL DEFAULT '', UNIQUE(curriculum_id,subject_id,year_level)
);
CREATE TABLE faculty (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE RESTRICT,
 name TEXT NOT NULL, employment TEXT NOT NULL DEFAULT '', consideration TEXT NOT NULL DEFAULT '',
 max_weekly_minutes INTEGER NOT NULL DEFAULT 0 CHECK(max_weekly_minutes BETWEEN 0 AND 5040),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE sections (
 id INTEGER PRIMARY KEY, program_id INTEGER NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
 curriculum_id INTEGER NOT NULL REFERENCES curricula(id) ON DELETE RESTRICT,
 period_id INTEGER NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
 code TEXT NOT NULL COLLATE NOCASE, year_level INTEGER NOT NULL CHECK(year_level BETWEEN 1 AND 6),
 size INTEGER NOT NULL DEFAULT 40 CHECK(size BETWEEN 0 AND 1000),
 adviser_id INTEGER REFERENCES faculty(id) ON DELETE RESTRICT,
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(period_id,program_id,code)
);
CREATE TABLE faculty_subjects (
 id INTEGER PRIMARY KEY, faculty_id INTEGER NOT NULL REFERENCES faculty(id) ON DELETE RESTRICT,
 subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT, UNIQUE(faculty_id,subject_id)
);
CREATE TABLE teaching_assignments (
 id INTEGER PRIMARY KEY, faculty_id INTEGER NOT NULL REFERENCES faculty(id) ON DELETE RESTRICT,
 subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
 section_id INTEGER NOT NULL REFERENCES sections(id) ON DELETE RESTRICT,
 UNIQUE(section_id,subject_id),
 FOREIGN KEY(faculty_id,subject_id) REFERENCES faculty_subjects(faculty_id,subject_id) ON DELETE RESTRICT
);
CREATE TABLE faculty_work_times (
 id INTEGER PRIMARY KEY, faculty_id INTEGER NOT NULL REFERENCES faculty(id) ON DELETE RESTRICT,
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 6), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), UNIQUE(faculty_id,day)
);
CREATE TABLE faculty_considerations (
 id INTEGER PRIMARY KEY, faculty_id INTEGER NOT NULL REFERENCES faculty(id) ON DELETE RESTRICT,
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 6), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), note TEXT NOT NULL DEFAULT ''
);
CREATE TABLE buildings (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE RESTRICT,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'building' CHECK(kind IN('building','field')),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE rooms (
 id INTEGER PRIMARY KEY, building_id INTEGER NOT NULL REFERENCES buildings(id) ON DELETE RESTRICT,
 department_id INTEGER REFERENCES departments(id) ON DELETE RESTRICT,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'classroom' CHECK(kind IN('classroom','laboratory','field')),
 capacity INTEGER NOT NULL DEFAULT 40 CHECK(capacity BETWEEN 0 AND 1000),
 start_minute INTEGER NOT NULL DEFAULT 420 CHECK(start_minute>=0), end_minute INTEGER NOT NULL DEFAULT 1140 CHECK(end_minute>start_minute AND end_minute<=1440),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE generation_runs (
 id INTEGER PRIMARY KEY, department_id INTEGER NOT NULL REFERENCES departments(id), period_id INTEGER NOT NULL REFERENCES academic_periods(id),
 mode TEXT NOT NULL CHECK(mode IN('standard','force')), seed INTEGER NOT NULL, revision INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'preview' CHECK(status IN('preview','committed','replaced','released')),
 outcome TEXT NOT NULL CHECK(outcome IN('complete','partial')), result_json TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, committed_at TEXT, created_by INTEGER
);
CREATE TABLE scheduled_meetings (
 id INTEGER PRIMARY KEY, run_id INTEGER NOT NULL REFERENCES generation_runs(id),
 department_id INTEGER NOT NULL REFERENCES departments(id), period_id INTEGER NOT NULL REFERENCES academic_periods(id),
 section_id INTEGER NOT NULL REFERENCES sections(id), subject_id INTEGER NOT NULL REFERENCES subjects(id),
 faculty_id INTEGER NOT NULL REFERENCES faculty(id), room_id INTEGER NOT NULL REFERENCES rooms(id),
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 6), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), borrowed INTEGER NOT NULL CHECK(borrowed IN(0,1))
);
CREATE INDEX meetings_time ON scheduled_meetings(day,start_minute,end_minute);
CREATE INDEX meetings_period_department ON scheduled_meetings(period_id,department_id);
CREATE INDEX sections_period ON sections(period_id,program_id);
CREATE INDEX curriculum_lookup ON curriculum_subject_years(curriculum_id,year_level);
CREATE INDEX assignments_faculty ON teaching_assignments(faculty_id);
CREATE TABLE users (
 id INTEGER PRIMARY KEY, username TEXT NOT NULL COLLATE NOCASE UNIQUE, password_hash TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE sessions (
 token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 csrf_token TEXT NOT NULL, expires_at INTEGER NOT NULL
);
CREATE TABLE audit_events (
 id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), action TEXT NOT NULL, entity TEXT NOT NULL, entity_id INTEGER,
 detail TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- Defense in depth: simultaneous physical use is rejected even by direct inserts.
CREATE TRIGGER meeting_no_overlap BEFORE INSERT ON scheduled_meetings BEGIN
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM scheduled_meetings m JOIN academic_periods p ON p.id=m.period_id
  JOIN academic_periods n ON n.id=NEW.period_id
  WHERE m.day=NEW.day AND m.start_minute<NEW.end_minute AND NEW.start_minute<m.end_minute
  AND p.start_date<=n.end_date AND n.start_date<=p.end_date
  AND (m.room_id=NEW.room_id OR m.faculty_id=NEW.faculty_id OR m.section_id=NEW.section_id)
 ) THEN RAISE(ABORT,'room, faculty or section overlap') END;
END;
CREATE TRIGGER meeting_no_update BEFORE UPDATE ON scheduled_meetings BEGIN
 SELECT RAISE(ABORT,'committed meetings are immutable; replace the department schedule transactionally');
END;
CREATE TRIGGER revision_departments_insert AFTER INSERT ON departments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_departments_update AFTER UPDATE ON departments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_departments_delete AFTER DELETE ON departments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_programs_insert AFTER INSERT ON programs BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_programs_update AFTER UPDATE ON programs BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_programs_delete AFTER DELETE ON programs BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_academic_periods_insert AFTER INSERT ON academic_periods BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_academic_periods_update AFTER UPDATE ON academic_periods BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_academic_periods_delete AFTER DELETE ON academic_periods BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subjects_insert AFTER INSERT ON subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subjects_update AFTER UPDATE ON subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subjects_delete AFTER DELETE ON subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curricula_insert AFTER INSERT ON curricula BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curricula_update AFTER UPDATE ON curricula BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curricula_delete AFTER DELETE ON curricula BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curriculum_subject_years_insert AFTER INSERT ON curriculum_subject_years BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curriculum_subject_years_update AFTER UPDATE ON curriculum_subject_years BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_curriculum_subject_years_delete AFTER DELETE ON curriculum_subject_years BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_insert AFTER INSERT ON faculty BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_update AFTER UPDATE ON faculty BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_delete AFTER DELETE ON faculty BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_sections_insert AFTER INSERT ON sections BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_sections_update AFTER UPDATE ON sections BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_sections_delete AFTER DELETE ON sections BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_subjects_insert AFTER INSERT ON faculty_subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_subjects_update AFTER UPDATE ON faculty_subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_subjects_delete AFTER DELETE ON faculty_subjects BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_teaching_assignments_insert AFTER INSERT ON teaching_assignments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_teaching_assignments_update AFTER UPDATE ON teaching_assignments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_teaching_assignments_delete AFTER DELETE ON teaching_assignments BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_work_times_insert AFTER INSERT ON faculty_work_times BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_work_times_update AFTER UPDATE ON faculty_work_times BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_work_times_delete AFTER DELETE ON faculty_work_times BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_considerations_insert AFTER INSERT ON faculty_considerations BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_considerations_update AFTER UPDATE ON faculty_considerations BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_considerations_delete AFTER DELETE ON faculty_considerations BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_buildings_insert AFTER INSERT ON buildings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_buildings_update AFTER UPDATE ON buildings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_buildings_delete AFTER DELETE ON buildings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_rooms_insert AFTER INSERT ON rooms BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_rooms_update AFTER UPDATE ON rooms BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_rooms_delete AFTER DELETE ON rooms BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_scheduled_meetings_insert AFTER INSERT ON scheduled_meetings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_scheduled_meetings_update AFTER UPDATE ON scheduled_meetings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_scheduled_meetings_delete AFTER DELETE ON scheduled_meetings BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
`

func TestUpgradeRestoresSourceNamesWithoutReplacingLocalEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(legacySchemaForTest); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app_meta SET value='Anonymized reference dataset' WHERE key='dataset';
INSERT INTO departments(id,code,name) VALUES(20,'CCJE','COLLEGE OF CRIMINAL JUSTICE EDUCATION');
INSERT INTO departments(id,code,name) VALUES(22,'COHM','COLLEGE OF HOSPITALITY AND TOURISM EDUCATION');
INSERT INTO departments(id,code,name) VALUES(21,'CITE','COLLEGE OF INFORMATION TECHNOLOGY EDUCATION');
INSERT INTO departments(id,code,name) VALUES(24,'GENED','GENERAL EDUCATION / SHARED ACADEMIC SUBJECTS');
INSERT INTO departments(id,code,name) VALUES(25,'BSMT','COLLEGE OF MARITIME TRANSPORTATION EDUCATION');
INSERT INTO departments(id,code,name) VALUES(23,'COTE','COLLEGE OF TEACHER EDUCATION');
INSERT INTO faculty(id,department_id,name,consideration) VALUES(1,23,'Faculty 001','Anonymous reference profile. Confirm the name and availability before actual use.');
INSERT INTO faculty(id,department_id,name,consideration) VALUES(2,23,'Personal edited name','Anonymous reference profile. Confirm the name and availability before actual use.');
INSERT INTO faculty(id,department_id,name,consideration) VALUES(3,23,'Faculty 003','Anonymous reference profile. Confirm the name and availability before actual use.');
INSERT INTO faculty_work_times(id,faculty_id,day,start_minute,end_minute) VALUES(1,3,1,540,600);
INSERT INTO users(id,username,password_hash) VALUES(1,'existing_user','keep_this_hash');
INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at) VALUES('existing_session',1,'existing_csrf',9999999999);
INSERT INTO subjects(id,code,title,allowed_days) VALUES(1,'OLD','Legacy subject','weekdays_saturday');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path, schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	var name, hash string
	var sourceID, n, fallback int
	if err = s.DB.QueryRow("SELECT name,source_faculty_id FROM faculty WHERE id=1").Scan(&name, &sourceID); err != nil {
		t.Fatal(err)
	}
	if name == "Faculty 001" || sourceID != 49 {
		t.Fatal("source identity mapping was not repaired", sourceID)
	}
	s.DB.QueryRow("SELECT name FROM faculty WHERE id=2").Scan(&name)
	if name != "Personal edited name" {
		t.Fatal("edited name overwritten")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty").Scan(&n)
	if n != 3 {
		t.Fatal("deleted source records resurrected")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty_work_times WHERE faculty_id=3").Scan(&n)
	if n != 1 {
		t.Fatal("custom working week overwritten")
	}
	s.DB.QueryRow("SELECT start_minute FROM faculty_work_times WHERE faculty_id=3").Scan(&n)
	if n != 540 {
		t.Fatal("custom hours changed")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty_work_times WHERE faculty_id=1").Scan(&n)
	if n == 0 {
		t.Fatal("source working hours missing")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty_work_times WHERE faculty_id=2").Scan(&n)
	if n != 0 {
		t.Fatal("edited profile given unrequested working hours")
	}
	s.DB.QueryRow("SELECT password_hash FROM users WHERE id=1").Scan(&hash)
	if hash != "keep_this_hash" {
		t.Fatal("local account lost")
	}
	s.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&n)
	if n != 1 {
		t.Fatal("session lost during migration")
	}
	var mask string
	s.DB.QueryRow("SELECT allowed_days,saturday_fallback FROM subjects WHERE id=1").Scan(&mask, &fallback)
	if mask != "1,2,3,4,5" || fallback != 1 {
		t.Fatal("legacy day conversion", mask, fallback)
	}
	if _, err = s.DB.Exec("INSERT INTO faculty_work_times(faculty_id,day,start_minute,end_minute) VALUES(3,7,480,600)"); err != nil {
		t.Fatal("Sunday not supported after upgrade", err)
	}
	rs, err := s.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rs.Next() {
		t.Fatal("invalid upgrade FKs")
	}
	rs.Close()
	backup, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.db"))
	if err != nil || len(backup) != 1 {
		t.Fatal("missing pre-update backup", err)
	}
	if st, e := os.Stat(backup[0]); e != nil || st.Size() == 0 {
		t.Fatal("empty backup")
	}
	s.Close()
	s, err = Open(path, schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	backup, _ = filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.db"))
	if len(backup) != 1 {
		t.Fatal("repair repeated on normal restart")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty").Scan(&n)
	if n != 3 {
		t.Fatal("restart inserted source records")
	}
}

func TestUpgradePreservesReservationsAndConflictTriggers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(legacySchemaForTest); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
 UPDATE app_meta SET value='Anonymized reference dataset' WHERE key='dataset';
 INSERT INTO departments(id,code,name) VALUES(24,'GENED','General Education');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,24,'TEST','Test');
 INSERT INTO academic_periods(id,label,academic_year,semester,start_date,end_date) VALUES(1,'Legacy','2026','First','2026-08-01','2026-12-01');
 INSERT INTO subjects(id,code,title) VALUES(1,'SUB','Existing subject');
 INSERT INTO curricula(id,program_id,name) VALUES(1,1,'Existing curriculum');
 INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1);
 INSERT INTO faculty(id,department_id,name) VALUES(1,24,'Faculty 001');
 INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level) VALUES(1,1,1,1,'TEST-A',1);
 INSERT INTO faculty_subjects(id,faculty_id,subject_id) VALUES(1,1,1);
 INSERT INTO teaching_assignments(id,faculty_id,subject_id,section_id) VALUES(1,1,1,1);
 INSERT INTO buildings(id,code,name) VALUES(1,'B','Building');
 INSERT INTO rooms(id,building_id,code,name) VALUES(1,1,'R','Room');
 INSERT INTO generation_runs(id,department_id,period_id,mode,seed,revision,status,outcome,result_json) VALUES(1,24,1,'standard',42,0,'committed','partial','{}');
 INSERT INTO scheduled_meetings(id,run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed) VALUES(1,1,24,1,1,1,1,1,1,420,480,0);
 `)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path, schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	var status string
	s.DB.QueryRow("SELECT count(*) FROM scheduled_meetings WHERE id=1 AND start_minute=420 AND end_minute=480").Scan(&n)
	if n != 1 {
		t.Fatal("committed meeting changed")
	}
	s.DB.QueryRow("SELECT status FROM generation_runs WHERE id=1").Scan(&status)
	if status != "committed" {
		t.Fatal("run status changed")
	}
	s.DB.QueryRow("SELECT count(*) FROM faculty_work_times WHERE faculty_id=1").Scan(&n)
	if n != 0 {
		t.Fatal("existing reservation assigned new working constraints")
	}
	_, err = s.DB.Exec("INSERT INTO scheduled_meetings(run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed) VALUES(1,24,1,1,1,1,1,1,420,480,0)")
	if err == nil {
		t.Fatal("overlap trigger missing after migration")
	}
	if _, err = s.DB.Exec("UPDATE scheduled_meetings SET end_minute=600 WHERE id=1"); err == nil {
		t.Fatal("immutable meeting trigger missing")
	}
}
