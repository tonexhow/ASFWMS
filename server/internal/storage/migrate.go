package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A consistent SQLite backup is made before upgrading a user's existing file.
func backupBeforeUpgrade(db *sql.DB, path string) error {
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	name := filepath.Join(dir, "before-update-"+time.Now().UTC().Format("20060102T150405.000000000")+".db")
	_, err := db.Exec("VACUUM INTO '" + strings.ReplaceAll(name, "'", "''") + "'")
	if err == nil {
		_ = os.Chmod(name, 0600)
	}
	return err
}

// Rebuild changed tables using the checked-in schema. All IDs, existing records,
// auth data and reservations are copied. The old file remains usable on failure.
func migrateDaysAndPriority(db *sql.DB, schemaPath string) (err error) {
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
	type obj struct{ kind, name, statement string }
	objects := []obj{}
	for rs.Next() {
		var o obj
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
	for _, o := range objects {
		if _, err = tx.Exec("DROP " + o.kind + " \"" + o.name + "\""); err != nil {
			return err
		}
	}
	for _, table := range []string{"academic_periods", "subjects", "faculty", "faculty_work_times", "faculty_considerations", "scheduled_meetings"} {
		re := regexp.MustCompile(`(?s)CREATE TABLE ` + table + ` \(.*?\n\);`)
		definition := legacyV8DDL(re.FindString(string(schema)))
		if definition == "" {
			return fmt.Errorf("missing schema definition: %s", table)
		}
		temp := "migration_" + table
		if _, err = tx.Exec(strings.Replace(definition, "CREATE TABLE "+table, "CREATE TABLE "+temp, 1)); err != nil {
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
		oldSet := map[string]bool{}
		for _, c := range oldCols {
			oldSet[c] = true
		}
		cols := []string{}
		exprs := []string{}
		for _, c := range newCols {
			if !oldSet[c] {
				if table == "subjects" && c == "saturday_fallback" {
					cols = append(cols, c)
					exprs = append(exprs, "CASE WHEN allowed_days='weekdays_saturday' THEN 1 ELSE 0 END")
				}
				continue
			}
			cols = append(cols, c)
			expr := `"` + c + `"`
			if table == "subjects" && c == "allowed_days" {
				expr = "CASE allowed_days WHEN 'weekdays' THEN '1,2,3,4,5' WHEN 'weekdays_saturday' THEN '1,2,3,4,5' WHEN 'saturday' THEN '6' ELSE allowed_days END"
			}
			exprs = append(exprs, expr)
		}
		if _, err = tx.Exec("INSERT INTO " + temp + " (" + strings.Join(cols, ",") + ") SELECT " + strings.Join(exprs, ",") + " FROM " + table); err != nil {
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
	if _, err = tx.Exec("INSERT INTO schema_migrations(number) VALUES(2); UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision';"); err != nil {
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
		return fmt.Errorf("migration foreign key check failed; rolled back")
	}
	return tx.Commit()
}
func columnNames(tx *sql.Tx, table string) ([]string, error) {
	rs, e := tx.Query("PRAGMA table_info(" + table + ")")
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []string{}
	for rs.Next() {
		var id, notnull, pk int
		var name, typ string
		var def any
		if e = rs.Scan(&id, &name, &typ, &notnull, &def, &pk); e != nil {
			return nil, e
		}
		out = append(out, name)
	}
	return out, rs.Err()
}
