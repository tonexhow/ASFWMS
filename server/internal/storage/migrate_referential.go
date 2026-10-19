package storage

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// migrateReferentialSafety upgrades existing databases to the explicit link policy.
// Optional parent references become ON DELETE SET NULL; association rows and
// immutable generated schedules keep RESTRICT so the reviewed application flow
// must resolve them first. The startup path creates a backup before calling us.
func migrateReferentialSafety(db *sql.DB, schemaPath string) (err error) {
	source, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	schema := legacyV8DDL(string(source))
	if _, err = db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer func() {
		_, enableErr := db.Exec("PRAGMA foreign_keys=ON")
		if err == nil {
			err = enableErr
		}
	}()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save all existing application-defined indexes and triggers before rebuilding.
	// These include scheduling conflict and revision triggers, which must survive.
	rs, err := tx.Query("SELECT type,name,sql FROM sqlite_master WHERE type IN ('trigger','index') AND sql IS NOT NULL")
	if err != nil {
		return err
	}
	type ddlObject struct{ kind, name, statement string }
	objects := []ddlObject{}
	for rs.Next() {
		var o ddlObject
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
	// Remove indexes before triggers to ensure reliable rebuild across SQLite versions.
	sort.SliceStable(objects, func(i, j int) bool { return objects[i].kind < objects[j].kind })
	for _, o := range objects {
		if _, err = tx.Exec("DROP " + o.kind + ` "` + o.name + `"`); err != nil {
			return err
		}
	}

	tables := []string{"programs", "subjects", "curricula", "faculty", "sections", "buildings", "rooms"}
	for _, table := range tables {
		re := regexp.MustCompile(`(?s)CREATE TABLE ` + table + ` \(.*?\n\);`)
		ddl := re.FindString(schema)
		if ddl == "" {
			return fmt.Errorf("safe relationships: schema missing %s", table)
		}
		if table == "curricula" {
			ddl = curriculumDDLBeforeScope(ddl)
		}
		temp := "referential_safety_" + table
		ddl = strings.Replace(ddl, "CREATE TABLE "+table, "CREATE TABLE "+temp, 1)
		if _, err = tx.Exec(ddl); err != nil {
			return fmt.Errorf("create %s: %w", table, err)
		}
		oldCols, e := columnNames(tx, table)
		if e != nil {
			return e
		}
		newCols, e := columnNames(tx, temp)
		if e != nil {
			return e
		}
		oldSet := make(map[string]bool, len(oldCols))
		for _, c := range oldCols {
			oldSet[c] = true
		}
		if len(oldCols) != len(newCols) {
			return fmt.Errorf("unexpected column count in %s; rollback without data changes", table)
		}
		for _, c := range newCols {
			if !oldSet[c] {
				return fmt.Errorf("unexpected column %s in %s; rollback without data changes", c, table)
			}
		}
		// Older migrations appended columns in a different order. Copy by
		// explicit COLUMN NAME, never by positional SELECT *.
		cols := make([]string, 0, len(newCols))
		for _, c := range newCols {
			cols = append(cols, `"`+c+`"`)
		}
		joined := strings.Join(cols, ",")
		if _, err = tx.Exec("INSERT INTO " + temp + " (" + joined + ") SELECT " + joined + " FROM " + table); err != nil {
			return fmt.Errorf("copy %s: %w", table, err)
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
			return fmt.Errorf("restore %s %s: %w", o.kind, o.name, err)
		}
	}
	// Use the SQL in schema.sql as the single source of truth for new indexes and guards.
	indexDefs := regexp.MustCompile(`(?m)^CREATE INDEX idx_[A-Za-z0-9_]+ .*?;$`).FindAllString(schema, -1)
	if len(indexDefs) < 12 {
		return fmt.Errorf("schema missing referential indexes")
	}
	for _, ddl := range indexDefs {
		if strings.Contains(ddl, "idx_subject_components_") {
			continue
		}
		if _, err = tx.Exec(ddl); err != nil {
			return fmt.Errorf("create safety index: %w", err)
		}
	}
	names := []string{
		"guard_section_context_update", "guard_membership_delete", "guard_membership_update",
		"guard_faculty_eligibility_delete", "guard_instructor_assignment_delete",
	}
	for _, name := range names {
		re := regexp.MustCompile(`(?s)CREATE TRIGGER ` + name + ` .*?END;`)
		ddl := re.FindString(schema)
		if ddl == "" {
			return fmt.Errorf("schema missing trigger %s", name)
		}
		if _, err = tx.Exec(ddl); err != nil {
			return fmt.Errorf("create trigger %s: %w", name, err)
		}
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations(number) VALUES(5)"); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE app_meta SET value=CAST(value AS INTEGER)+1 WHERE key='revision'"); err != nil {
		return err
	}
	checks, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	invalid := checks.Next()
	checkErr := checks.Err()
	checks.Close()
	if checkErr != nil {
		return checkErr
	}
	if invalid {
		return fmt.Errorf("foreign key check failed; migration rolled back")
	}
	return tx.Commit()
}
