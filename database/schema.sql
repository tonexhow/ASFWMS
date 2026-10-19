-- ASFWMS local schema. Numbered schema state lives in schema_migrations.
-- This file creates a NEW database. Startup never overwrites an existing one.
PRAGMA foreign_keys = ON;
CREATE TABLE schema_migrations (number INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
INSERT INTO schema_migrations(number) VALUES (1),(2),(3),(4),(5),(6),(7),(8);
INSERT INTO schema_migrations(number) VALUES(9),(10);
CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO app_meta VALUES ('revision','0'),('dataset','empty');
CREATE TABLE departments (
 id INTEGER PRIMARY KEY, code TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK(length(trim(code))>0),
 name TEXT NOT NULL CHECK(length(trim(name))>0), active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE programs (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE academic_periods (
 id INTEGER PRIMARY KEY, label TEXT NOT NULL UNIQUE, academic_year TEXT NOT NULL, semester TEXT NOT NULL, term TEXT NOT NULL DEFAULT '',
 start_date TEXT NOT NULL DEFAULT '', end_date TEXT NOT NULL DEFAULT '' CHECK(end_date='' OR start_date='' OR end_date>=start_date), active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)),
 day_start INTEGER NOT NULL DEFAULT 420 CHECK(day_start>=0 AND day_start<1440),
 day_end INTEGER NOT NULL DEFAULT 1140 CHECK(day_end>day_start AND day_end<=1440),
 lunch_start INTEGER NOT NULL DEFAULT 660 CHECK(lunch_start>=0 AND lunch_start<1440),
 lunch_end INTEGER NOT NULL DEFAULT 840 CHECK(lunch_end>lunch_start AND lunch_end<=1440),
 lunch_minutes INTEGER NOT NULL DEFAULT 60 CHECK(lunch_minutes>=0 AND lunch_minutes<=lunch_end-lunch_start),
 max_consecutive INTEGER NOT NULL DEFAULT 3 CHECK(max_consecutive BETWEEN 0 AND 3)
);
CREATE TABLE subjects (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL,
 code TEXT NOT NULL COLLATE NOCASE, title TEXT NOT NULL, units REAL NOT NULL DEFAULT 3 CHECK(units>=0),
 weekly_minutes INTEGER NOT NULL DEFAULT 180 CHECK(weekly_minutes>0 AND weekly_minutes<=3600),
 priority TEXT NOT NULL DEFAULT 'major' CHECK(priority IN('major','minor')),
 required_room TEXT NOT NULL DEFAULT 'any' CHECK(required_room IN('any','classroom','laboratory','field')),
 pattern TEXT NOT NULL DEFAULT 'auto' CHECK(pattern IN('auto','mwf','tth','mw','mf','wf','sat','sun','single','flex')),
 allowed_days TEXT NOT NULL DEFAULT '1,2,3,4,5' CHECK(length(allowed_days)>0 AND allowed_days NOT GLOB '*[^1-7,]*'),
 saturday_fallback INTEGER NOT NULL DEFAULT 0 CHECK(saturday_fallback IN(0,1)),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(department_id,code)
);
CREATE TABLE curricula (
 id INTEGER PRIMARY KEY, program_id INTEGER REFERENCES programs(id) ON DELETE SET NULL,
 name TEXT NOT NULL,
 year_level INTEGER NOT NULL DEFAULT 0 CHECK(year_level BETWEEN 0 AND 6),
 semester TEXT NOT NULL DEFAULT '' CHECK(semester IN('','1st Semester','2nd Semester','Summer','Others')),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(program_id,name),
 CHECK((year_level=0 AND semester='') OR (year_level>0 AND semester<>''))
);
CREATE TABLE curriculum_subject_years (
 id INTEGER PRIMARY KEY, curriculum_id INTEGER NOT NULL REFERENCES curricula(id) ON DELETE RESTRICT,
 subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
 year_level INTEGER NOT NULL CHECK(year_level BETWEEN 1 AND 6),
 source_semester TEXT NOT NULL DEFAULT '', UNIQUE(curriculum_id,subject_id,year_level)
);
CREATE TABLE faculty (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL,
 name TEXT NOT NULL, faculty_id_number TEXT NOT NULL DEFAULT '', prefix TEXT NOT NULL DEFAULT '', first_name TEXT NOT NULL DEFAULT '', middle_name TEXT NOT NULL DEFAULT '', last_name TEXT NOT NULL DEFAULT '', extension TEXT NOT NULL DEFAULT '', email TEXT NOT NULL DEFAULT '', employment TEXT NOT NULL DEFAULT '', consideration TEXT NOT NULL DEFAULT '',
 priority_scheduling INTEGER NOT NULL DEFAULT 0 CHECK(priority_scheduling IN(0,1)),
 section_policy TEXT NOT NULL DEFAULT 'priority' CHECK(section_policy IN('priority','exclusive')),
 source_faculty_id INTEGER UNIQUE,
 max_weekly_minutes INTEGER NOT NULL DEFAULT 0 CHECK(max_weekly_minutes BETWEEN 0 AND 5040),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
-- Blank faculty ID numbers are allowed; populated values must be unique.
CREATE UNIQUE INDEX faculty_id_number_unique ON faculty(faculty_id_number COLLATE NOCASE) WHERE faculty_id_number <> '';
CREATE TABLE sections (
 id INTEGER PRIMARY KEY, program_id INTEGER REFERENCES programs(id) ON DELETE SET NULL,
 curriculum_id INTEGER REFERENCES curricula(id) ON DELETE SET NULL,
 period_id INTEGER REFERENCES academic_periods(id) ON DELETE SET NULL,
 code TEXT NOT NULL COLLATE NOCASE, year_level INTEGER NOT NULL CHECK(year_level BETWEEN 1 AND 6),
 size INTEGER NOT NULL DEFAULT 50 CHECK(size BETWEEN 0 AND 1000),
 adviser_id INTEGER REFERENCES faculty(id) ON DELETE SET NULL,
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), UNIQUE(period_id,program_id,year_level,code)
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
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 7), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), UNIQUE(faculty_id,day)
);
-- Soft faculty placement preferences. A zero primary window means no time preference;
-- secondary preference may only be set if the primary window is populated.
CREATE TABLE faculty_preferences (
 faculty_id INTEGER PRIMARY KEY REFERENCES faculty(id) ON DELETE CASCADE,
 first_start_minute INTEGER NOT NULL DEFAULT 0,
 first_end_minute INTEGER NOT NULL DEFAULT 0,
 second_start_minute INTEGER NOT NULL DEFAULT 0,
 second_end_minute INTEGER NOT NULL DEFAULT 0,
 preferred_room_id INTEGER REFERENCES rooms(id) ON DELETE SET NULL,
 CHECK((first_start_minute=0 AND first_end_minute=0) OR (first_start_minute>=420 AND first_end_minute>first_start_minute AND first_end_minute<=1140)),
 CHECK((second_start_minute=0 AND second_end_minute=0) OR (first_end_minute>0 AND second_start_minute>=420 AND second_end_minute>second_start_minute AND second_end_minute<=1140))
);
CREATE TABLE faculty_considerations (
 id INTEGER PRIMARY KEY, faculty_id INTEGER NOT NULL REFERENCES faculty(id) ON DELETE RESTRICT,
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 7), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), note TEXT NOT NULL DEFAULT ''
);
CREATE TABLE buildings (
 id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'building' CHECK(kind IN('building','field')),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1))
);
CREATE TABLE rooms (
 id INTEGER PRIMARY KEY, building_id INTEGER REFERENCES buildings(id) ON DELETE SET NULL,
 department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL,
 code TEXT NOT NULL COLLATE NOCASE UNIQUE, name TEXT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'classroom' CHECK(kind IN('classroom','laboratory','field')),
 capacity INTEGER NOT NULL DEFAULT 50 CHECK(capacity BETWEEN 0 AND 1000),
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
CREATE TABLE subject_components (
 id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN('lecture','laboratory','field')),
 units REAL NOT NULL CHECK(units>0 AND units<=60),
 weekly_minutes INTEGER NOT NULL CHECK(weekly_minutes>0 AND weekly_minutes<=3600 AND weekly_minutes%15=0),
 sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_subject_components_subject ON subject_components(subject_id,sort_order,id);
CREATE TABLE subject_facility_requests (
 id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
 room_id INTEGER NOT NULL REFERENCES rooms(id) ON DELETE RESTRICT, UNIQUE(subject_id,room_id)
);
CREATE TABLE generation_waits (
 id INTEGER PRIMARY KEY, department_id INTEGER NOT NULL REFERENCES departments(id),
 period_id INTEGER NOT NULL REFERENCES academic_periods(id),
 source_run_id INTEGER NOT NULL REFERENCES generation_runs(id),
 latest_run_id INTEGER REFERENCES generation_runs(id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN('pending','ready','resolved','cancelled')),
 requested_by INTEGER, detail TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_generation_waits_active ON generation_waits(department_id,period_id) WHERE status IN('pending','ready');
CREATE TRIGGER revision_subject_components_insert AFTER INSERT ON subject_components BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subject_components_update AFTER UPDATE ON subject_components BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subject_components_delete AFTER DELETE ON subject_components BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subject_facility_requests_insert AFTER INSERT ON subject_facility_requests BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subject_facility_requests_update AFTER UPDATE ON subject_facility_requests BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_subject_facility_requests_delete AFTER DELETE ON subject_facility_requests BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;

CREATE TABLE scheduled_meetings (
 id INTEGER PRIMARY KEY, component_id INTEGER REFERENCES subject_components(id), component_kind TEXT NOT NULL DEFAULT '', run_id INTEGER NOT NULL REFERENCES generation_runs(id),
 department_id INTEGER NOT NULL REFERENCES departments(id), period_id INTEGER NOT NULL REFERENCES academic_periods(id),
 section_id INTEGER NOT NULL REFERENCES sections(id), subject_id INTEGER NOT NULL REFERENCES subjects(id),
 faculty_id INTEGER NOT NULL REFERENCES faculty(id), room_id INTEGER NOT NULL REFERENCES rooms(id),
 day INTEGER NOT NULL CHECK(day BETWEEN 1 AND 7), start_minute INTEGER NOT NULL CHECK(start_minute>=0),
 end_minute INTEGER NOT NULL CHECK(end_minute>start_minute AND end_minute<=1440), borrowed INTEGER NOT NULL CHECK(borrowed IN(0,1))
);
-- Foreign-key lookup indexes keep deletion previews and safe detachment responsive.
CREATE INDEX idx_programs_department ON programs(department_id);
CREATE INDEX idx_subjects_department ON subjects(department_id);
CREATE INDEX idx_curricula_program ON curricula(program_id);
-- Scoped curricula are unique for one program/year/semester, even while empty.
-- Legacy curricula (year_level=0) retain their existing multi-year membership.
CREATE UNIQUE INDEX idx_curricula_scoped_unique ON curricula(program_id,year_level,semester)
 WHERE active=1 AND program_id IS NOT NULL AND year_level>0;
-- Sections are semester-independent bank records. period_id and curriculum_id
-- remain nullable legacy links for compatibility with historical imports.
-- A new section is stored with both NULL; generation resolves curriculum from
-- program + year + selected academic period's semester.
CREATE UNIQUE INDEX idx_sections_bank_unique ON sections(program_id,year_level,code)
 WHERE period_id IS NULL AND program_id IS NOT NULL;
CREATE INDEX idx_sections_bank_lookup ON sections(program_id,year_level,active);
CREATE INDEX idx_sections_curriculum_year ON sections(curriculum_id,year_level);
CREATE INDEX idx_sections_adviser ON sections(adviser_id);
CREATE INDEX idx_faculty_department ON faculty(department_id);
CREATE INDEX idx_faculty_subjects_subject ON faculty_subjects(subject_id);
CREATE INDEX idx_teaching_assignments_faculty_subject ON teaching_assignments(faculty_id,subject_id);
CREATE INDEX idx_teaching_assignments_section ON teaching_assignments(section_id);
CREATE INDEX idx_faculty_considerations_faculty ON faculty_considerations(faculty_id);
CREATE INDEX idx_buildings_department ON buildings(department_id);
CREATE INDEX idx_rooms_building ON rooms(building_id);
CREATE INDEX idx_rooms_department ON rooms(department_id);
CREATE INDEX idx_runs_department_period ON generation_runs(department_id,period_id);
CREATE INDEX idx_runs_period ON generation_runs(period_id);
CREATE INDEX idx_meetings_run ON scheduled_meetings(run_id);
CREATE INDEX idx_meetings_section ON scheduled_meetings(section_id);
CREATE INDEX idx_meetings_faculty ON scheduled_meetings(faculty_id);
CREATE INDEX idx_meetings_room ON scheduled_meetings(room_id);
CREATE INDEX meetings_time ON scheduled_meetings(day,start_minute,end_minute);
CREATE INDEX meetings_period_department ON scheduled_meetings(period_id,department_id);
CREATE INDEX sections_period ON sections(period_id,program_id);
CREATE INDEX curriculum_lookup ON curriculum_subject_years(curriculum_id,year_level);
CREATE INDEX assignments_faculty ON teaching_assignments(faculty_id);
CREATE TABLE users (
 id INTEGER PRIMARY KEY, username TEXT NOT NULL COLLATE NOCASE UNIQUE, password_hash TEXT NOT NULL,
 name TEXT NOT NULL DEFAULT '', email TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT 'scheduler' CHECK(role IN ('developer','administrator','scheduler')),
 department_id INTEGER REFERENCES departments(id) ON DELETE RESTRICT, protected INTEGER NOT NULL DEFAULT 0 CHECK(protected IN (0,1)),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1)), created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_users_department ON users(department_id);
CREATE TABLE password_reset_tokens (
 token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at INTEGER NOT NULL, used INTEGER NOT NULL DEFAULT 0 CHECK(used IN (0,1)),
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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
-- Relational safety: an occupied section may not be silently reassigned to a
-- different curriculum / program / period without releasing its schedule first.
CREATE TRIGGER guard_section_context_update BEFORE UPDATE OF program_id,curriculum_id,period_id ON sections
WHEN (OLD.program_id IS NOT NEW.program_id OR OLD.curriculum_id IS NOT NEW.curriculum_id OR OLD.period_id IS NOT NEW.period_id)
 AND EXISTS (SELECT 1 FROM scheduled_meetings WHERE section_id=OLD.id)
BEGIN SELECT RAISE(ABORT,'release section timetable before changing its academic context'); END;
-- Memberships are links, but removing a currently scheduled subject directly
-- in SQLite would make the timetable inconsistent. Use the reviewed app flow.
CREATE TRIGGER guard_membership_delete BEFORE DELETE ON curriculum_subject_years
WHEN EXISTS (
 SELECT 1 FROM sections s JOIN scheduled_meetings m ON m.section_id=s.id
 WHERE s.curriculum_id=OLD.curriculum_id AND s.year_level=OLD.year_level AND m.subject_id=OLD.subject_id
)
BEGIN SELECT RAISE(ABORT,'release timetable before unlinking a scheduled curriculum subject'); END;
CREATE TRIGGER guard_membership_update BEFORE UPDATE OF curriculum_id,subject_id,year_level ON curriculum_subject_years
WHEN (OLD.curriculum_id IS NOT NEW.curriculum_id OR OLD.subject_id IS NOT NEW.subject_id OR OLD.year_level IS NOT NEW.year_level)
 AND EXISTS (
 SELECT 1 FROM sections s JOIN scheduled_meetings m ON m.section_id=s.id
 WHERE s.curriculum_id=OLD.curriculum_id AND s.year_level=OLD.year_level AND m.subject_id=OLD.subject_id
)
BEGIN SELECT RAISE(ABORT,'release timetable before changing a scheduled curriculum subject'); END;
CREATE TRIGGER guard_faculty_eligibility_delete BEFORE DELETE ON faculty_subjects
WHEN EXISTS (SELECT 1 FROM scheduled_meetings WHERE faculty_id=OLD.faculty_id AND subject_id=OLD.subject_id)
BEGIN SELECT RAISE(ABORT,'release timetable before unlinking scheduled faculty eligibility'); END;
CREATE TRIGGER guard_instructor_assignment_delete BEFORE DELETE ON teaching_assignments
WHEN EXISTS (SELECT 1 FROM scheduled_meetings WHERE faculty_id=OLD.faculty_id AND subject_id=OLD.subject_id AND section_id=OLD.section_id)
BEGIN SELECT RAISE(ABORT,'release timetable before deleting a scheduled instructor assignment'); END;
CREATE TRIGGER meeting_no_overlap BEFORE INSERT ON scheduled_meetings BEGIN
 SELECT CASE WHEN EXISTS (
  SELECT 1 FROM scheduled_meetings m JOIN academic_periods p ON p.id=m.period_id
  JOIN academic_periods n ON n.id=NEW.period_id
  WHERE m.day=NEW.day AND m.start_minute<NEW.end_minute AND NEW.start_minute<m.end_minute
  AND p.academic_year=n.academic_year AND p.semester=n.semester
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
CREATE TRIGGER revision_faculty_preferences_insert AFTER INSERT ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_preferences_update AFTER UPDATE ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
CREATE TRIGGER revision_faculty_preferences_delete AFTER DELETE ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END;
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
