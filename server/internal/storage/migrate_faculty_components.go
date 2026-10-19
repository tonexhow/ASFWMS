package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed migration_v9.sql
var migrationV9 string

// All changes, including new defaults, are atomic; Open backs up the database
// first. Existing 40-seat records are intentionally preserved; only 45 -> 50.
func migrateFacultyComponents(db *sql.DB) (err error) {
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
	type object struct{ kind, name, ddl string }
	objects := []object{}
	rs, err := tx.Query("SELECT type,name,sql FROM sqlite_master WHERE type IN('trigger','index') AND sql IS NOT NULL")
	if err != nil {
		return err
	}
	for rs.Next() {
		var o object
		if err = rs.Scan(&o.kind, &o.name, &o.ddl); err != nil {
			rs.Close()
			return err
		}
		objects = append(objects, o)
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return err
	}
	for _, o := range objects {
		if _, err = tx.Exec("DROP " + strings.ToUpper(o.kind) + ` "` + o.name + `"`); err != nil {
			return err
		}
	}
	for _, table := range []string{"sections", "rooms"} {
		var ddl string
		if err = tx.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&ddl); err != nil {
			return err
		}
		temp := table + "_v9"
		ddl = regexp.MustCompile(`(?i)^CREATE TABLE\s+"?`+table+`"?\s*\(`).ReplaceAllString(ddl, "CREATE TABLE "+temp+" (")
		ddl = regexp.MustCompile(`(?i)(size|capacity) INTEGER NOT NULL DEFAULT [0-9]+`).ReplaceAllString(ddl, "${1} INTEGER NOT NULL DEFAULT 50")
		if _, err = tx.Exec(ddl); err != nil {
			return fmt.Errorf("create %s: %w", temp, err)
		}
		cols, e := columnNames(tx, table)
		if e != nil {
			return e
		}
		names := strings.Join(cols, ",")
		for _, stmt := range []string{"INSERT INTO " + temp + " (" + names + ") SELECT " + names + " FROM " + table, "DROP TABLE " + table, "ALTER TABLE " + temp + " RENAME TO " + table} {
			if _, err = tx.Exec(stmt); err != nil {
				return err
			}
		}
	}
	for _, o := range objects {
		if _, err = tx.Exec(o.ddl); err != nil {
			return fmt.Errorf("restore %s: %w", o.name, err)
		}
	}
	if _, err = tx.Exec(migrationV9); err != nil {
		return err
	}
	check, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := check.Next()
	err = check.Err()
	check.Close()
	if err != nil {
		return err
	}
	if bad {
		return fmt.Errorf("foreign key inconsistency; migration rolled back")
	}
	return tx.Commit()
}

// Earlier upgrades rebuild tables from schema.sql. Keep their old column shape
// until v9 runs, avoiding duplicate ADD COLUMN statements and missing FK tables.
func legacyV8DDL(ddl string) string {
	for _, col := range []string{"prefix", "first_name", "middle_name", "last_name", "extension", "email"} {
		ddl = strings.ReplaceAll(ddl, col+" TEXT NOT NULL DEFAULT '', ", "")
	}
	ddl = strings.ReplaceAll(ddl, "component_id INTEGER REFERENCES subject_components(id), ", "")
	ddl = strings.ReplaceAll(ddl, "component_kind TEXT NOT NULL DEFAULT '', ", "")
	return ddl
}
