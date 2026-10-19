package storage

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// migrateSafeLinks permits an existing section, curriculum, program or room to
// survive deletion of a referenced record. No original rows or IDs are dropped.
// The caller creates a consistent backup before invoking this migration.
func migrateSafeLinks(db *sql.DB, schemaPath string) (err error) {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
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
	rs, err := tx.Query("SELECT type,name,sql FROM sqlite_master WHERE type IN ('trigger','index') AND sql IS NOT NULL")
	if err != nil {
		return err
	}
	type object struct{ kind, name, statement string }
	objects := []object{}
	for rs.Next() {
		var o object
		if err = rs.Scan(&o.kind, &o.name, &o.statement); err != nil {
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
	// SQLite does not allow dropping NOT NULL directly. Rebuild the affected
	// tables in a single transaction, preserving existing rows and constraints.
	for _, o := range objects {
		if _, err = tx.Exec("DROP " + o.kind + ` "` + o.name + `"`); err != nil {
			return err
		}
	}
	for _, table := range []string{"programs", "curricula", "sections", "rooms"} {
		re := regexp.MustCompile(`(?s)CREATE TABLE ` + table + ` \(.*?\n\);`)
		def := re.FindString(string(schema))
		if def == "" {
			return fmt.Errorf("missing table definition for %s", table)
		}
		if table == "curricula" {
			def = curriculumDDLBeforeScope(def)
		}
		temp := "safe_links_" + table
		if _, err = tx.Exec(strings.Replace(def, "CREATE TABLE "+table, "CREATE TABLE "+temp, 1)); err != nil {
			return err
		}
		oldCols, e := columnNames(tx, table)
		if e != nil {
			return e
		}
		newCols, e := columnNames(tx, temp)
		if e != nil {
			return e
		}
		if strings.Join(oldCols, ",") != strings.Join(newCols, ",") {
			return fmt.Errorf("unexpected schema for %s; migration stopped before changes", table)
		}
		columns := make([]string, 0, len(oldCols))
		for _, c := range oldCols {
			columns = append(columns, `"`+c+`"`)
		}
		joined := strings.Join(columns, ",")
		if _, err = tx.Exec("INSERT INTO " + temp + " (" + joined + ") SELECT " + joined + " FROM " + table); err != nil {
			return err
		}
		if _, err = tx.Exec("DROP TABLE " + table); err != nil {
			return err
		}
		if _, err = tx.Exec("ALTER TABLE " + temp + " RENAME TO " + table); err != nil {
			return err
		}
	}
	for _, o := range objects {
		if _, err = tx.Exec(o.statement); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations(number) VALUES (4); UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'"); err != nil {
		return err
	}
	checks, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	failed := checks.Next()
	scanErr := checks.Err()
	checks.Close()
	if scanErr != nil {
		return scanErr
	}
	if failed {
		return fmt.Errorf("foreign key violation after safe-link migration")
	}
	return tx.Commit()
}
