package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// migrateCurriculumScope preserves legacy multi-year curricula (year_level=0)
// while allowing new curricula to be explicitly scoped by year and semester.
// Open creates an on-disk backup before running this transactional migration.
func migrateCurriculumScope(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"ALTER TABLE curricula ADD COLUMN year_level INTEGER NOT NULL DEFAULT 0 CHECK(year_level BETWEEN 0 AND 6)",
		"ALTER TABLE curricula ADD COLUMN semester TEXT NOT NULL DEFAULT '' CHECK(semester IN('','1st Semester','2nd Semester','Summer','Others'))",
		"CREATE UNIQUE INDEX idx_curricula_scoped_unique ON curricula(program_id,year_level,semester) WHERE active=1 AND program_id IS NOT NULL AND year_level>0",
		"INSERT INTO schema_migrations(number) VALUES(6)",
		"UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'",
	} {
		if _, err = tx.Exec(statement); err != nil {
			return fmt.Errorf("curriculum scope migration: %w", err)
		}
	}
	bad, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	violates := bad.Next()
	scanErr := bad.Err()
	bad.Close()
	if scanErr != nil {
		return scanErr
	}
	if violates {
		return fmt.Errorf("curriculum scope migration found foreign key violation")
	}
	return tx.Commit()
}

// curriculumDDLBeforeScope removes version-6-only fields when the version-4
// and version-5 migrations rebuild curricula from schema.sql. Version 6 adds
// the fields transactionally after those upgrades complete.
func curriculumDDLBeforeScope(ddl string) string {
	ddl = strings.Replace(ddl, "\n year_level INTEGER NOT NULL DEFAULT 0 CHECK(year_level BETWEEN 0 AND 6),", "", 1)
	ddl = strings.Replace(ddl, "\n semester TEXT NOT NULL DEFAULT '' CHECK(semester IN('','1st Semester','2nd Semester','Summer','Others')),", "", 1)
	ddl = strings.Replace(ddl, ",\n CHECK((year_level=0 AND semester='') OR (year_level>0 AND semester<>''))", "", 1)
	return ddl
}
