package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"net/http"
)

func (a *App) workTimes(w http.ResponseWriter, r *http.Request) {
	id, e := idOf(r)
	if e != nil {
		problem(w, 400, "ID", e.Error())
		return
	}
	var in struct {
		UseDefault bool `json:"use_default"`
		Days       []struct {
			Day   int `json:"day"`
			Start int `json:"start_minute"`
			End   int `json:"end_minute"`
		} `json:"days"`
	}
	if e = decode(r, &in); e != nil || len(in.Days) > 7 {
		problem(w, 400, "WORK_TIME", "Send up to seven working days.")
		return
	}
	if !in.UseDefault && len(in.Days) == 0 {
		problem(w, 400, "WORK_TIME", "Choose at least one working day or explicitly use the default.")
		return
	}
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		exists := false
		for _, f := range s.Faculty {
			if f.ID == id {
				exists = true
			}
		}
		if !exists {
			return errors.New("faculty member not found")
		}
		if e = domain.GuardChanges(s, "faculty_work_times", 0, nil, map[string]any{"faculty_id": id}, true); e != nil {
			return e
		}
		if _, e = q.ExecContext(r.Context(), "DELETE FROM faculty_work_times WHERE faculty_id=?", id); e != nil {
			return e
		}
		seen := map[int]bool{}
		if !in.UseDefault {
			for _, d := range in.Days {
				if d.Day < 1 || d.Day > 7 || d.Start < 0 || d.End > 1440 || d.Start >= d.End || seen[d.Day] {
					return errors.New("invalid or duplicate working-day window")
				}
				seen[d.Day] = true
				if _, e = q.ExecContext(r.Context(), "INSERT INTO faculty_work_times(faculty_id,day,start_minute,end_minute) VALUES(?,?,?,?)", id, d.Day, d.Start, d.End); e != nil {
					return e
				}
			}
		}
		return audit(r.Context(), q, user(r), "work_times", "faculty", id, "Working-day availability updated")
	})
	if e != nil {
		problem(w, 409, "WORK_TIME", safeError(e))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
