package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Developer-only, all-or-nothing section bank creation. No advisers are
// assigned; duplicates are scoped by program + year + section code.
func (a *App) developerBatchSections(w http.ResponseWriter, r *http.Request) {
	if user(r).Role != "developer" {
		deny(w)
		return
	}
	var in struct {
		ProgramID int      `json:"program_id"`
		YearLevel int      `json:"year_level"`
		Codes     []string `json:"codes"`
		Size      int      `json:"size"`
	}
	if err := decode(r, &in); err != nil || in.ProgramID <= 0 || in.YearLevel < 1 || in.YearLevel > 6 || len(in.Codes) == 0 || len(in.Codes) > 100 || in.Size < 0 || in.Size > 1000 {
		problem(w, 400, "BATCH", "Choose a program, year level and 1-100 section codes; size must be between 0 and 1000.")
		return
	}
	names := make([]string, 0, len(in.Codes))
	seen := map[string]bool{}
	for _, raw := range in.Codes {
		code := strings.TrimSpace(raw)
		key := strings.ToLower(code)
		if code == "" || len([]rune(code)) > 100 || seen[key] {
			problem(w, 400, "BATCH", "Section codes must be non-empty, at most 100 characters and unique within this batch.")
			return
		}
		seen[key] = true
		names = append(names, code)
	}
	added := 0
	err := a.Store.Write(r.Context(), func(q storage.Querier) error {
		if err := checkRevision(r, q); err != nil {
			return err
		}
		s, _, err := domain.Load(r.Context(), q)
		if err != nil {
			return err
		}
		found := false
		for _, pr := range s.Programs {
			if pr.ID == in.ProgramID && pr.Active == 1 {
				found = true
				break
			}
		}
		if !found {
			return errors.New("choose an active academic program")
		}
		for _, sec := range s.Sections {
			if sec.ProgramID != in.ProgramID || sec.Year != in.YearLevel {
				continue
			}
			if seen[strings.ToLower(strings.TrimSpace(sec.Code))] {
				return fmt.Errorf("section %s already exists in this program and year", sec.Code)
			}
		}
		size := in.Size
		for _, code := range names {
			_, err = q.ExecContext(r.Context(), "INSERT INTO sections(program_id,code,year_level,size,adviser_id,active) VALUES(?,?,?,?,NULL,1)", in.ProgramID, code, in.YearLevel, size)
			if err != nil {
				return fmt.Errorf("section %s: %w", code, err)
			}
			added++
		}
		return audit(r.Context(), q, user(r), "batch-create", "sections", 0, fmt.Sprintf("Created %d %s sections for program %d", added, domain.YearLabel(in.YearLevel), in.ProgramID))
	})
	if err != nil {
		problem(w, 409, "BATCH", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]int{"added": added})
}
