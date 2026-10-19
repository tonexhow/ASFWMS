package httpapi

import (
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"net/http"
)

func (a *App) sectionPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Policy string `json:"policy"`
	}
	if err = decode(r, &in); err != nil || (in.Policy != "priority" && in.Policy != "exclusive") {
		problem(w, 400, "POLICY", "Choose priority or exclusive.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var n int
		if err := q.QueryRowContext(r.Context(), "SELECT count(*) FROM faculty WHERE id=?", id).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return errors.New("faculty member not found")
		}
		if _, err := q.ExecContext(r.Context(), "UPDATE faculty SET section_policy=? WHERE id=?", in.Policy, id); err != nil {
			return err
		}
		return audit(r.Context(), q, user(r), "section-policy", "faculty", id, "Faculty assignment policy updated to "+in.Policy)
	})
	if err != nil {
		problem(w, 409, "POLICY", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
