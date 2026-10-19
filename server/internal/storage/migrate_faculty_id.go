package storage

import (
	"database/sql"
	"fmt"
)

// migrateFacultyID adds a user-supplied faculty identifier without touching the
// internal row IDs used by assignments and scheduled meetings. SQLite rolls the
// entire change back if any step fails.
func migrateFacultyID(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"ALTER TABLE faculty ADD COLUMN faculty_id_number TEXT NOT NULL DEFAULT ''",
		"CREATE UNIQUE INDEX faculty_id_number_unique ON faculty(faculty_id_number COLLATE NOCASE) WHERE faculty_id_number <> ''",
		"INSERT INTO schema_migrations(number) VALUES(10)",
		"UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'",
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("faculty ID migration: %w", err)
		}
	}
	return tx.Commit()
}
