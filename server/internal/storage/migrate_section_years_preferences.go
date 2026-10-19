package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// Version 8 restores section-code reuse across different year levels in one
// program, preserving all section IDs, existing foreign keys and triggers.
// Every existing database is backed up before this atomic migration runs.
func migrateSectionYearsAndPreferences(db *sql.DB) (err error) {
	if _, err = db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer func() {
		_, e := db.Exec("PRAGMA foreign_keys=ON")
		if err == nil {
			err = e
		}
	}()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT type,name,sql FROM sqlite_master WHERE type IN ('index','trigger') AND sql IS NOT NULL")
	if err != nil {
		return err
	}
	type schemaObject struct{ kind, name, ddl string }
	objects := []schemaObject{}
	for rows.Next() {
		var o schemaObject
		if err = rows.Scan(&o.kind, &o.name, &o.ddl); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Other tables have triggers that reference sections. Temporarily remove
	// every application index and trigger before rebuilding the table, then
	// restore all their original SQL inside this same atomic transaction.
	for _, object := range objects {
		kind := strings.ToUpper(object.kind)
		if _, err = tx.Exec("DROP " + kind + ` "` + object.name + `"`); err != nil {
			return fmt.Errorf("temporarily drop %s: %w", object.name, err)
		}
	}
	// Create replacement with the same columns/IDs and a year-aware legacy
	// uniqueness rule, instead of editing the original in place.
	commands := []string{
		`CREATE TABLE sections_v8 (
          id INTEGER PRIMARY KEY, program_id INTEGER REFERENCES programs(id) ON DELETE SET NULL,
          curriculum_id INTEGER REFERENCES curricula(id) ON DELETE SET NULL,
          period_id INTEGER REFERENCES academic_periods(id) ON DELETE SET NULL,
          code TEXT NOT NULL COLLATE NOCASE, year_level INTEGER NOT NULL CHECK(year_level BETWEEN 1 AND 6),
          size INTEGER NOT NULL DEFAULT 50 CHECK(size BETWEEN 0 AND 1000),
          adviser_id INTEGER REFERENCES faculty(id) ON DELETE SET NULL,
          active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)),
          UNIQUE(period_id,program_id,year_level,code))`,
		`INSERT INTO sections_v8(id,program_id,curriculum_id,period_id,code,year_level,size,adviser_id,active)
          SELECT id,program_id,curriculum_id,period_id,code,year_level,size,adviser_id,active FROM sections`,
		`DROP TABLE sections`,
		`ALTER TABLE sections_v8 RENAME TO sections`,
	}
	for _, stmt := range commands {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("rebuild section bank: %w", err)
		}
	}
	for _, object := range objects {
		stmt := object.ddl
		if object.name == "idx_sections_bank_unique" {
			stmt = strings.Replace(stmt, "sections(program_id,code)", "sections(program_id,year_level,code)", 1)
		}
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("restore section %s: %w", object.name, err)
		}
	}
	for _, stmt := range []string{
		`CREATE TABLE faculty_preferences (
          faculty_id INTEGER PRIMARY KEY REFERENCES faculty(id) ON DELETE CASCADE,
          first_start_minute INTEGER NOT NULL DEFAULT 0,
          first_end_minute INTEGER NOT NULL DEFAULT 0,
          second_start_minute INTEGER NOT NULL DEFAULT 0,
          second_end_minute INTEGER NOT NULL DEFAULT 0,
          preferred_room_id INTEGER REFERENCES rooms(id) ON DELETE SET NULL,
          CHECK((first_start_minute=0 AND first_end_minute=0) OR (first_start_minute>=420 AND first_end_minute>first_start_minute AND first_end_minute<=1140)),
          CHECK((second_start_minute=0 AND second_end_minute=0) OR (first_end_minute>0 AND second_start_minute>=420 AND second_end_minute>second_start_minute AND second_end_minute<=1140)))`,
		`CREATE TRIGGER revision_faculty_preferences_insert AFTER INSERT ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END`,
		`CREATE TRIGGER revision_faculty_preferences_update AFTER UPDATE ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END`,
		`CREATE TRIGGER revision_faculty_preferences_delete AFTER DELETE ON faculty_preferences BEGIN UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'; END`,
		`INSERT INTO schema_migrations(number) VALUES(8)`,
		`UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'`,
	} {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("preferences migration: %w", err)
		}
	}
	fk, e := tx.Query("PRAGMA foreign_key_check")
	if e != nil {
		return e
	}
	bad := fk.Next()
	checkErr := fk.Err()
	fk.Close()
	if checkErr != nil {
		return checkErr
	}
	if bad {
		return fmt.Errorf("section migration found foreign key inconsistencies; no changes saved")
	}
	return tx.Commit()
}
