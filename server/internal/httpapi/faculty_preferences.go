package httpapi

import (
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"net/http"
)

// Time placement is driven only by weekly availability and unavailable windows.
// Old clients may send the retired time fields, but they are ignored and cleared.
func (a *App) facultyPreferences(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		PreferredRoomID int `json:"preferred_room_id"`
		FirstStart      int `json:"first_start_minute"`
		FirstEnd        int `json:"first_end_minute"`
		SecondStart     int `json:"second_start_minute"`
		SecondEnd       int `json:"second_end_minute"`
	}
	if decode(r, &in) != nil || in.PreferredRoomID < 0 {
		problem(w, 400, "FACILITY", "Choose a facility or leave it blank.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		var n int
		if e := q.QueryRowContext(r.Context(), "SELECT count(*) FROM faculty WHERE id=?", id).Scan(&n); e != nil {
			return e
		}
		if n == 0 {
			return errors.New("faculty member not found")
		}
		var room any
		if in.PreferredRoomID > 0 {
			if e := q.QueryRowContext(r.Context(), "SELECT count(*) FROM rooms r JOIN buildings b ON b.id=r.building_id WHERE r.id=? AND r.active=1 AND b.active=1", in.PreferredRoomID).Scan(&n); e != nil {
				return e
			}
			if n == 0 {
				return errors.New("choose an active facility")
			}
			room = in.PreferredRoomID
		}
		_, e := q.ExecContext(r.Context(), `INSERT INTO faculty_preferences(faculty_id,preferred_room_id) VALUES(?,?) ON CONFLICT(faculty_id) DO UPDATE SET preferred_room_id=excluded.preferred_room_id,first_start_minute=0,first_end_minute=0,second_start_minute=0,second_end_minute=0`, id, room)
		if e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "facility", "faculty", id, "Optional facility preference changed; weekly hours unchanged")
	})
	if err != nil {
		problem(w, 409, "FACILITY", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
