package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/scheduling"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (a *App) readGeneration(ctx context.Context, id int) (scheduling.Result, string, error) {
	var payload, status string
	if err := a.Store.DB.QueryRowContext(ctx, "SELECT result_json,status FROM generation_runs WHERE id=?", id).Scan(&payload, &status); err != nil {
		return scheduling.Result{}, "", errors.New("generation run not found")
	}
	var result scheduling.Result
	err := json.Unmarshal([]byte(payload), &result)
	return result, status, err
}
func (a *App) prepareContinuation(r *http.Request, in *scheduling.Request, s domain.Snapshot) error {
	if (in.Mode == "force" || in.BorrowPolicy == "force") && in.ConfirmBorrow != "FORCE" {
		return errors.New("confirm FORCE before allowing foreign facilities; existing reservations will remain protected")
	}
	if in.BorrowPolicy == "force" {
		in.Mode = "force"
	}
	if in.BorrowPolicy == "wait" {
		in.Mode = "standard"
	}
	if len(in.FacilityChoices) > 1000 {
		return errors.New("too many facility choices")
	}
	seen := map[[2]int]bool{}
	for _, choice := range in.FacilityChoices {
		key := [2]int{choice.SectionID, choice.SubjectID}
		if seen[key] || s.DepartmentOfSection(choice.SectionID) != in.DepartmentID || !s.SectionRequires(choice.SectionID, choice.SubjectID) || len(choice.RoomIDs) == 0 || len(choice.RoomIDs) > 1000 {
			return errors.New("invalid or duplicate section-subject facility choice")
		}
		seen[key] = true
		for _, id := range choice.RoomIDs {
			room := getRoom(s, id)
			if id < 1 || room.Active != 1 || getBuilding(s, room.BuildingID).Active != 1 {
				return errors.New("choose active facilities from active buildings")
			}
		}
	}
	if in.SourceRunID == 0 {
		return nil
	}
	source, _, err := a.readGeneration(r.Context(), in.SourceRunID)
	if err != nil {
		return err
	}
	if source.Request.DepartmentID != in.DepartmentID || source.Request.PeriodID != in.PeriodID {
		return errors.New("continuation must use the same department and academic period")
	}
	in.Pinned = source.Meetings
	return nil
}
func (a *App) persistPreview(ctx context.Context, in scheduling.Request, result scheduling.Result, revision int64, createdBy int) (int64, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return 0, err
	}
	var id int64
	err = a.Store.Write(ctx, func(q storage.Querier) error {
		now, err := storage.Revision(ctx, q)
		if err != nil {
			return err
		}
		if now != revision {
			return errors.New("records or reservations changed while generating; regenerate against the latest data")
		}
		res, err := q.ExecContext(ctx, "INSERT INTO generation_runs(department_id,period_id,mode,seed,revision,outcome,result_json,created_by) VALUES(?,?,?,?,?,?,?,?)", in.DepartmentID, in.PeriodID, in.Mode, in.Seed, revision, result.Outcome, string(payload), createdBy)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}
func (a *App) facilityCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	result, _, err := a.readGeneration(r.Context(), id)
	if err != nil {
		problem(w, 404, "RUN", err.Error())
		return
	}
	section, _ := strconv.Atoi(r.URL.Query().Get("section"))
	subject, _ := strconv.Atoi(r.URL.Query().Get("subject"))
	s, revision, err := a.snapshot(r.Context())
	if err != nil {
		problem(w, 503, "DATABASE", safeError(err))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	candidates, err := scheduling.FacilityOptions(ctx, s, result, section, subject)
	if err != nil {
		problem(w, 409, "FACILITY", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"candidates": candidates, "revision": revision, "pending_departments": scheduling.PendingDepartments(s, result.Request.DepartmentID, result.Request.PeriodID)})
}
func (a *App) waitForFacilities(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var input struct {
		Confirm string `json:"confirm"`
	}
	if decode(r, &input) != nil || input.Confirm != "WAIT" {
		problem(w, 400, "CONFIRM", "Confirm WAIT to queue this department's unresolved facilities.")
		return
	}
	result, status, err := a.readGeneration(r.Context(), id)
	if err != nil {
		problem(w, 404, "RUN", err.Error())
		return
	}
	if status != "preview" && status != "committed" {
		problem(w, 409, "RUN", "Only a current preview or committed schedule can wait.")
		return
	}
	shortage := false
	for _, issue := range result.Issues {
		if issue.Code == "LACK_OF_FACILITY" || issue.Code == "LACK_FACILITY" || issue.Code == "NO_COMPATIBLE_ROOM" {
			shortage = true
		}
	}
	if !shortage {
		problem(w, 409, "WAIT", "This run has no unresolved facility shortages.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		res, err := q.ExecContext(r.Context(), "UPDATE generation_waits SET source_run_id=?,latest_run_id=NULL,status='pending',requested_by=?,detail='',updated_at=CURRENT_TIMESTAMP WHERE department_id=? AND period_id=? AND status IN('pending','ready')", id, user(r).ID, result.Request.DepartmentID, result.Request.PeriodID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			_, err = q.ExecContext(r.Context(), "INSERT INTO generation_waits(department_id,period_id,source_run_id,requested_by) VALUES(?,?,?,?)", result.Request.DepartmentID, result.Request.PeriodID, id, user(r).ID)
		}
		return err
	})
	if err != nil {
		problem(w, 409, "WAIT", safeError(err))
		return
	}
	go a.processWaits()
	writeJSON(w, 200, map[string]any{"ok": true, "message": "Waiting for all other active section-owning departments to commit complete schedules. ASFWMS will prepare a new preview, never auto-commit it."})
}
func (a *App) listWaits(w http.ResponseWriter, r *http.Request) {
	query := "SELECT * FROM generation_waits WHERE status IN('pending','ready')"
	args := []any{}
	if user(r).Role == "scheduler" {
		query += " AND department_id=?"
		args = append(args, user(r).DepartmentID)
	}
	items, err := storage.ReadRows(r.Context(), a.Store.DB, query+" ORDER BY id DESC", args...)
	if err != nil {
		problem(w, 503, "WAIT", safeError(err))
		return
	}
	s, _, err := a.snapshot(r.Context())
	if err != nil {
		problem(w, 503, "WAIT", safeError(err))
		return
	}
	for _, v := range items {
		v["pending_departments"] = scheduling.PendingDepartments(s, domain.Int(v, "department_id"), domain.Int(v, "period_id"))
	}
	writeJSON(w, 200, map[string]any{"waits": items})
	// Polling can recover work after a server restart. A mutex and final status
	// check make the preparation idempotent across simultaneous clients.
	go a.processWaits()
}
func (a *App) cancelWait(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if decode(r, &in) != nil || in.Confirm != "CANCEL" {
		problem(w, 400, "CONFIRM", "Confirm CANCEL to stop waiting.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var dep int
		if err := q.QueryRowContext(r.Context(), "SELECT department_id FROM generation_waits WHERE id=?", id).Scan(&dep); err != nil {
			return errors.New("wait request not found")
		}
		if user(r).Role == "scheduler" && dep != user(r).DepartmentID {
			return errors.New("wait request belongs to another department")
		}
		_, err := q.ExecContext(r.Context(), "UPDATE generation_waits SET status='cancelled',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN('pending','ready')", id)
		return err
	})
	if err != nil {
		problem(w, 409, "WAIT", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) processWaits() {
	if !a.waitMu.TryLock() {
		return
	}
	defer a.waitMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pending, err := storage.ReadRows(ctx, a.Store.DB, "SELECT * FROM generation_waits WHERE status='pending' ORDER BY id")
	if err != nil {
		return
	}
	for _, item := range pending {
		if ctx.Err() != nil {
			return
		}
		s, rev, err := a.snapshot(ctx)
		if err != nil {
			return
		}
		dept, period := domain.Int(item, "department_id"), domain.Int(item, "period_id")
		if len(scheduling.PendingDepartments(s, dept, period)) > 0 {
			continue
		}
		source, _, err := a.readGeneration(ctx, domain.Int(item, "source_run_id"))
		if err != nil {
			continue
		}
		in := source.Request
		in.SourceRunID = domain.Int(item, "source_run_id")
		in.Mode = "standard"
		in.BorrowPolicy = "wait"
		in.ConfirmBorrow = ""
		in.Pinned = source.Meetings
		attempt, done := context.WithTimeout(ctx, 25*time.Second)
		result, err := scheduling.Generate(attempt, s, in)
		done()
		if err == nil {
			// Status, source and revision are rechecked atomically so cancelling
			// or replacing the wait while generation runs cannot resurrect it.
			var payload []byte
			payload, err = json.Marshal(result)
			if err == nil {
				err = a.Store.Write(ctx, func(q storage.Querier) error {
					var status string
					var sourceID int
					if err := q.QueryRowContext(ctx, "SELECT status,source_run_id FROM generation_waits WHERE id=?", item["id"]).Scan(&status, &sourceID); err != nil {
						return err
					}
					if status != "pending" || sourceID != in.SourceRunID {
						return errors.New("wait changed during generation")
					}
					now, err := storage.Revision(ctx, q)
					if err != nil {
						return err
					}
					if now != rev {
						return errors.New("reservations changed; waiting preview will retry")
					}
					res, err := q.ExecContext(ctx, "INSERT INTO generation_runs(department_id,period_id,mode,seed,revision,outcome,result_json,created_by) VALUES(?,?,?,?,?,?,?,?)", dept, period, in.Mode, in.Seed, rev, result.Outcome, string(payload), item["requested_by"])
					if err != nil {
						return err
					}
					id, err := res.LastInsertId()
					if err != nil {
						return err
					}
					_, err = q.ExecContext(ctx, "UPDATE generation_waits SET status='ready',latest_run_id=?,detail=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", id, fmt.Sprintf("Prepared %d/%d subject-section placements; review before committing.", result.Placed, result.Required), item["id"])
					return err
				})
			}
		}
		if err != nil {
			_ = a.Store.Write(ctx, func(q storage.Querier) error {
				_, e := q.ExecContext(ctx, "UPDATE generation_waits SET detail=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='pending'", safeError(err), item["id"])
				return e
			})
		}
	}
}
