package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Store struct {
	DB       *sql.DB
	SeedPath string
}

func Open(path, schema string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	// A single connection plus rollback-journal mode keeps laptop file transfer simple.
	if _, err = db.Exec("PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=DELETE;"); err != nil {
		return fail(err)
	}
	var fk int
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return fail(errors.New("SQLite foreign keys could not be enabled"))
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&n); err != nil {
		return fail(err)
	}
	if n == 0 {
		text, err := os.ReadFile(schema)
		if err != nil {
			return fail(err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		if _, err = tx.Exec(string(text)); err != nil {
			tx.Rollback()
			return fail(fmt.Errorf("initialize schema: %w", err))
		}
		if err = tx.Commit(); err != nil {
			return fail(err)
		}
	} else {
		var v int
		if err = db.QueryRow("SELECT max(number) FROM schema_migrations").Scan(&v); err != nil || (v < 1 || v > 9) {
			return fail(errors.New("unrecognized database schema; no data changed; use a matching build and keep a backup"))
		}
		var dataset string
		if err = db.QueryRow("SELECT value FROM app_meta WHERE key='dataset'").Scan(&dataset); err != nil {
			return fail(err)
		}
		if v < 9 || dataset == "Anonymized reference dataset" {
			if err = backupBeforeUpgrade(db, path); err != nil {
				return fail(fmt.Errorf("pre-update backup failed: %w", err))
			}
		}
		if v == 1 {
			if err = migrateDaysAndPriority(db, schema); err != nil {
				return fail(fmt.Errorf("schema upgrade failed: %w", err))
			}
		}
		if v < 3 {
			if err = migrateAccessAndPeriods(db); err != nil {
				return fail(fmt.Errorf("access and period schema upgrade failed: %w", err))
			}
		}
		if v < 4 {
			if err = migrateSafeLinks(db, schema); err != nil {
				return fail(fmt.Errorf("safe-link schema upgrade failed: %w", err))
			}
		}
		if v < 5 {
			if err = migrateReferentialSafety(db, schema); err != nil {
				return fail(fmt.Errorf("referential safety schema upgrade failed: %w", err))
			}
		}
		if v < 6 {
			if err = migrateCurriculumScope(db); err != nil {
				return fail(fmt.Errorf("curriculum scope schema upgrade failed: %w", err))
			}
		}
		if v < 7 {
			if err = migrateSectionBank(db); err != nil {
				return fail(fmt.Errorf("section bank schema upgrade failed: %w", err))
			}
		}
		if v < 8 {
			if err = migrateSectionYearsAndPreferences(db); err != nil {
				return fail(fmt.Errorf("section year/preferences schema upgrade failed: %w", err))
			}
		}
		if v < 9 {
			if err = migrateFacultyComponents(db); err != nil {
				return fail(fmt.Errorf("faculty/components upgrade failed: %w", err))
			}
		}
		if dataset == "Anonymized reference dataset" {
			s := &Store{DB: db, SeedPath: filepath.Join(filepath.Dir(schema), "seed.sql")}
			if err = s.restoreSourceReference(context.Background()); err != nil {
				return fail(fmt.Errorf("reference update failed: %w", err))
			}
		}
	}
	_ = os.Chmod(path, 0600)
	return &Store{DB: db, SeedPath: filepath.Join(filepath.Dir(schema), "seed.sql")}, nil
}
func (s *Store) Close() error { return s.DB.Close() }

// Write obtains the SQLite RESERVED lock before reading any commit prerequisites.
func (s *Store) Write(ctx context.Context, fn func(Querier) error) error {
	c, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err = c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_, _ = c.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err = fn(c); err != nil {
		return err
	}
	if _, err = c.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	done = true
	return nil
}
func ReadRows(ctx context.Context, q Querier, statement string, args ...any) ([]map[string]any, error) {
	rows, err := q.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vs := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vs {
			ptrs[i] = &vs[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := map[string]any{}
		for i, c := range cols {
			if b, ok := vs[i].([]byte); ok {
				r[c] = string(b)
			} else {
				r[c] = vs[i]
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func Revision(ctx context.Context, q Querier) (int64, error) {
	var s string
	err := q.QueryRowContext(ctx, "SELECT value FROM app_meta WHERE key='revision'").Scan(&s)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(s, 10, 64)
}
func DecodeRows[T any](rows []map[string]any) ([]T, error) {
	b, e := json.Marshal(rows)
	if e != nil {
		return nil, e
	}
	var out []T
	e = json.Unmarshal(b, &out)
	return out, e
}

// Identifiers only come from domain allowlists, never directly from request text.
func Insert(ctx context.Context, q Querier, table string, data map[string]any, columns []string) (int64, error) {
	vals := []any{}
	qs := []string{}
	for _, c := range columns {
		vals = append(vals, data[c])
		qs = append(qs, "?")
	}
	r, e := q.ExecContext(ctx, "INSERT INTO "+table+" ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(qs, ",")+")", vals...)
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
