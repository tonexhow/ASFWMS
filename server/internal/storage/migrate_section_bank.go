package storage

import (
	"database/sql"
	"fmt"
)

// migrateSectionBank adds indexes without rewriting existing section rows.
// Older period/curriculum foreign keys remain as optional historical references.
// All new Sections are term-neutral, and the scheduler resolves their curricula
// for each selected period. This migration runs only after a database backup.
func migrateSectionBank(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sections_bank_unique ON sections(program_id,code)
         WHERE period_id IS NULL AND program_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_sections_bank_lookup ON sections(program_id,year_level,active)`,
		`INSERT INTO schema_migrations(number) VALUES(7)`,
		`UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("section bank migration: %w", err)
		}
	}
	rs, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := rs.Next()
	scanErr := rs.Err()
	rs.Close()
	if scanErr != nil {
		return scanErr
	}
	if bad {
		return fmt.Errorf("section bank migration foreign-key check failed")
	}
	return tx.Commit()
}
