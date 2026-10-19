package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// migrateAccessAndPeriods retains the original rows and account credentials;
// old installations acquire roles and rule defaults without discarding saved schedules.
func migrateAccessAndPeriods(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		"ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'scheduler'",
		"ALTER TABLE users ADD COLUMN department_id INTEGER REFERENCES departments(id)",
		"ALTER TABLE users ADD COLUMN protected INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE faculty ADD COLUMN section_policy TEXT NOT NULL DEFAULT 'priority'",
		"CREATE TABLE IF NOT EXISTS password_reset_tokens (token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at INTEGER NOT NULL, used INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)",
		"UPDATE users SET role='developer', protected=1 WHERE id=(SELECT id FROM users ORDER BY id LIMIT 1)",
		"UPDATE academic_periods SET lunch_minutes=60 WHERE lunch_minutes=0",
		"UPDATE academic_periods SET max_consecutive=3 WHERE max_consecutive=0 OR max_consecutive>3",
		"DROP TRIGGER IF EXISTS meeting_no_overlap",
		`CREATE TRIGGER meeting_no_overlap BEFORE INSERT ON scheduled_meetings BEGIN
    SELECT CASE WHEN EXISTS (
     SELECT 1 FROM scheduled_meetings m JOIN academic_periods p ON p.id=m.period_id JOIN academic_periods n ON n.id=NEW.period_id
     WHERE m.day=NEW.day AND m.start_minute<NEW.end_minute AND NEW.start_minute<m.end_minute
     AND p.academic_year=n.academic_year AND p.semester=n.semester
     AND (m.room_id=NEW.room_id OR m.faculty_id=NEW.faculty_id OR m.section_id=NEW.section_id)
    ) THEN RAISE(ABORT,'room, faculty or section overlap') END;
   END`,
		"INSERT INTO schema_migrations(number) VALUES(3)",
		"UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'",
	}
	for _, q := range stmts {
		if _, err = tx.Exec(q); err != nil {
			if strings.HasPrefix(q, "ALTER TABLE") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migration step %q: %w", q, err)
		}
	}
	return tx.Commit()
}
