package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/scheduling"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (a *App) snapshot(ctx context.Context) (domain.Snapshot, int64, error) {
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return domain.Snapshot{}, 0, e
	}
	defer tx.Rollback()
	s, _, e := domain.Load(ctx, tx)
	if e != nil {
		return s, 0, e
	}
	rev, e := storage.Revision(ctx, tx)
	if e != nil {
		return s, 0, e
	}
	e = tx.Commit()
	return s, rev, e
}
func (a *App) readiness(w http.ResponseWriter, r *http.Request) {
	var in scheduling.Request
	if e := decode(r, &in); e != nil {
		problem(w, 400, "INPUT", e.Error())
		return
	}
	s, _, e := a.snapshot(r.Context())
	if e != nil {
		problem(w, 503, "DATABASE", safeError(e))
		return
	}
	v, e := scheduling.Preflight(s, in)
	if e != nil {
		problem(w, 400, "READINESS", e.Error())
		return
	}
	writeJSON(w, 200, v)
}
func (a *App) generate(w http.ResponseWriter, r *http.Request) {
	var in scheduling.Request
	if err := decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	if in.Seed == 0 {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			problem(w, 500, "SEED", "Cannot create generation seed.")
			return
		}
		in.Seed = int64(binary.LittleEndian.Uint32(b[:]))
	}
	snap, rev, err := a.snapshot(r.Context())
	if err != nil {
		problem(w, 503, "DATABASE", safeError(err))
		return
	}
	if err = a.prepareContinuation(r, &in, snap); err != nil {
		problem(w, 400, "GENERATION", err.Error())
		return
	}
	streaming := r.URL.Path == "/api/generate/stream"
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var report func(scheduling.Progress)
	encoder := json.NewEncoder(w)
	emit := func(v any) {
		if err := encoder.Encode(v); err != nil {
			cancel()
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			cancel()
		}
	}
	if streaming {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(200)
		report = func(p scheduling.Progress) { emit(p) }
	}
	fail := func(status int, code, message string) {
		if streaming {
			emit(map[string]any{"type": "error", "error": map[string]string{"code": code, "message": message}})
		} else {
			problem(w, status, code, message)
		}
	}
	result, err := scheduling.GenerateWithProgress(ctx, snap, in, report)
	if err != nil {
		status := 400
		if errors.Is(err, context.DeadlineExceeded) {
			status = 408
		}
		fail(status, "GENERATION", err.Error())
		return
	}
	id, err := a.persistPreview(r.Context(), in, result, rev, user(r).ID)
	if err != nil {
		fail(409, "PREVIEW_STALE", safeError(err))
		return
	}
	response := map[string]any{"id": id, "revision": rev, "status": "preview", "result": result}
	if streaming {
		emit(map[string]any{"type": "result", "run": response})
	} else {
		writeJSON(w, 201, response)
	}
}
func (a *App) getRun(w http.ResponseWriter, r *http.Request) {
	id, e := idOf(r)
	if e != nil {
		problem(w, 400, "ID", e.Error())
		return
	}
	rs, e := storage.ReadRows(r.Context(), a.Store.DB, "SELECT * FROM generation_runs WHERE id=?", id)
	if e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	if len(rs) == 0 {
		problem(w, 404, "RUN", "Generation run not found.")
		return
	}
	run := rs[0]
	var result scheduling.Result
	if e = json.Unmarshal([]byte(run["result_json"].(string)), &result); e != nil {
		problem(w, 500, "RUN", "Stored preview cannot be read.")
		return
	}
	delete(run, "result_json")
	run["result"] = result
	writeJSON(w, 200, run)
}
func (a *App) commit(w http.ResponseWriter, r *http.Request) {
	id, e := idOf(r)
	if e != nil {
		problem(w, 400, "ID", e.Error())
		return
	}
	var in struct {
		Confirm      string `json:"confirm"`
		AllowPartial bool   `json:"allow_partial"`
	}
	if e = decode(r, &in); e != nil || in.Confirm != "COMMIT" {
		problem(w, 400, "CONFIRM", "Type COMMIT to save this preview.")
		return
	}
	already := false
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		rs, e := storage.ReadRows(r.Context(), q, "SELECT * FROM generation_runs WHERE id=?", id)
		if e != nil {
			return e
		}
		if len(rs) == 0 {
			return errors.New("preview not found")
		}
		run := rs[0]
		if run["status"] == "committed" {
			already = true
			return nil
		}
		if run["status"] != "preview" {
			return errors.New("historical runs cannot be recommitted; generate a fresh preview")
		}
		now, e := storage.Revision(r.Context(), q)
		if e != nil {
			return e
		}
		if now != int64(domain.Int(run, "revision")) {
			return errors.New("this preview is stale because records or reservations changed; generate again before committing")
		}
		var result scheduling.Result
		if e = json.Unmarshal([]byte(run["result_json"].(string)), &result); e != nil {
			return e
		}
		if result.Outcome == "partial" && !in.AllowPartial {
			return errors.New("this is a partial result; explicitly acknowledge unresolved requirements")
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		if e = scheduling.Validate(s, result); e != nil {
			return e
		}
		dept, period := result.Request.DepartmentID, result.Request.PeriodID
		if _, e = q.ExecContext(r.Context(), "DELETE FROM scheduled_meetings WHERE department_id=? AND period_id=?", dept, period); e != nil {
			return e
		}
		if _, e = q.ExecContext(r.Context(), "UPDATE generation_runs SET status='replaced' WHERE department_id=? AND period_id=? AND status='committed'", dept, period); e != nil {
			return e
		}
		for _, m := range result.Meetings {
			if _, e = q.ExecContext(r.Context(), "INSERT INTO scheduled_meetings(run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed,component_id,component_kind) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", id, m.DepartmentID, m.PeriodID, m.SectionID, m.SubjectID, m.FacultyID, m.RoomID, m.Day, m.Start, m.End, m.Borrowed, nullableID(m.ComponentID), m.ComponentKind); e != nil {
				return e
			}
		}
		if _, e = q.ExecContext(r.Context(), "UPDATE generation_runs SET status='committed',committed_at=CURRENT_TIMESTAMP WHERE id=?", id); e != nil {
			return e
		}
		if _, e = q.ExecContext(r.Context(), "UPDATE generation_waits SET status='resolved',updated_at=CURRENT_TIMESTAMP WHERE department_id=? AND period_id=? AND (latest_run_id=? OR ?='complete') AND status IN('pending','ready')", dept, period, id, result.Outcome); e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "commit", "generation_runs", id, fmt.Sprintf("%d meetings committed; %d borrowed occurrences", len(result.Meetings), result.Borrowed))
	})
	if e != nil {
		problem(w, 409, "COMMIT_BLOCKED", safeError(e))
		return
	}
	go a.processWaits()
	writeJSON(w, 200, map[string]any{"ok": true, "already_committed": already, "id": id})
}
func (a *App) release(w http.ResponseWriter, r *http.Request) {
	id, e := idOf(r)
	if e != nil {
		problem(w, 400, "ID", e.Error())
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if e = decode(r, &in); e != nil || in.Confirm != "RELEASE" {
		problem(w, 400, "CONFIRM", "Type RELEASE to remove this run's active reservations.")
		return
	}
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		var status string
		if e := q.QueryRowContext(r.Context(), "SELECT status FROM generation_runs WHERE id=?", id).Scan(&status); e != nil {
			return errors.New("run not found")
		}
		if status == "released" {
			return nil
		}
		if status != "committed" {
			return errors.New("only an active committed run can be released")
		}
		if _, e := q.ExecContext(r.Context(), "DELETE FROM scheduled_meetings WHERE run_id=?", id); e != nil {
			return e
		}
		if _, e := q.ExecContext(r.Context(), "UPDATE generation_runs SET status='released' WHERE id=?", id); e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "release", "generation_runs", id, "Reservations released explicitly; historical preview retained")
	})
	if e != nil {
		problem(w, 409, "RELEASE_BLOCKED", safeError(e))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func nullableID(id int) any {
	if id == 0 {
		return nil
	}
	return id
}
