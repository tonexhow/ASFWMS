package storage

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"strings"
)

//go:embed reference_manifest.json
var ManifestJSON []byte
var SeedOrder = []string{"departments", "programs", "academic_periods", "subjects", "curricula", "curriculum_subject_years", "faculty", "sections", "faculty_subjects", "teaching_assignments", "faculty_work_times", "faculty_considerations", "buildings", "rooms"}

// seed.sql is the only preload source, kept separate from schema.sql and Go code.
func (s *Store) seedPart(section string) (string, error) {
	text, e := os.ReadFile(s.SeedPath)
	if e != nil {
		return "", e
	}
	start := "-- BEGIN " + section
	end := "-- END " + section
	a := strings.Index(string(text), start)
	b := strings.Index(string(text), end)
	if a < 0 || b < a {
		return "", errors.New("invalid seed.sql: missing " + section + " block")
	}
	return string(text)[a+len(start) : b], nil
}
func (s *Store) Seed(ctx context.Context) error {
	text, err := s.seedPart("PRELOAD")
	if err != nil {
		return err
	}
	return s.Write(ctx, func(q Querier) error {
		for _, table := range SeedOrder {
			var n int
			if e := q.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); e != nil {
				return e
			}
			if n != 0 {
				return errors.New("preload refused: academic/faculty/facility data already exists; nothing was changed")
			}
		}
		_, e := q.ExecContext(ctx, text)
		return e
	})
}

// Executed once only for the earlier, identified anonymized reference workspace.
// The SQL repairs exact placeholders, never replaces a user-edited real name,
// and imports hours only for unchanged source profiles with no custom windows
// and no committed reservations. It does not insert missing/deleted entities.
func (s *Store) restoreSourceReference(ctx context.Context) error {
	text, e := s.seedPart("REFERENCE_REPAIR")
	if e != nil {
		return e
	}
	return s.Write(ctx, func(q Querier) error {
		_, e := q.ExecContext(ctx, text)
		return e
	})
}
