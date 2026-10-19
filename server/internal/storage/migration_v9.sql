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
ALTER TABLE faculty ADD COLUMN prefix TEXT NOT NULL DEFAULT '';
ALTER TABLE faculty ADD COLUMN first_name TEXT NOT NULL DEFAULT '';
ALTER TABLE faculty ADD COLUMN middle_name TEXT NOT NULL DEFAULT '';
ALTER TABLE faculty ADD COLUMN last_name TEXT NOT NULL DEFAULT '';
ALTER TABLE faculty ADD COLUMN extension TEXT NOT NULL DEFAULT '';
ALTER TABLE faculty ADD COLUMN email TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_meetings ADD COLUMN component_id INTEGER REFERENCES subject_components(id);
ALTER TABLE scheduled_meetings ADD COLUMN component_kind TEXT NOT NULL DEFAULT '';
INSERT INTO subject_components(subject_id,kind,units,weekly_minutes,sort_order)
 SELECT id, CASE required_room WHEN 'laboratory' THEN 'laboratory' WHEN 'field' THEN 'field' ELSE 'lecture' END,
 CASE WHEN units>0 THEN units ELSE weekly_minutes/60.0 END,weekly_minutes,0 FROM subjects
 WHERE weekly_minutes>0 AND weekly_minutes%15=0;
UPDATE faculty_preferences SET first_start_minute=0,first_end_minute=0,second_start_minute=0,second_end_minute=0;
UPDATE sections SET size=50 WHERE size=45;
UPDATE rooms SET capacity=50 WHERE capacity=45;
UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision';
INSERT INTO schema_migrations(number) VALUES(9);
