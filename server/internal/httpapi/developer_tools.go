package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var devTables = map[string]bool{
	"departments": true, "programs": true, "subjects": true, "curricula": true, "curriculum_subject_years": true,
	"sections": true, "faculty": true, "faculty_subjects": true, "teaching_assignments": true, "faculty_work_times": true,
	"faculty_considerations": true, "buildings": true, "rooms": true, "academic_periods": true,
	"scheduled_meetings": true, "generation_runs": true,
}

// No user credential, session, token, audit, schema migration or app setting
// can be accessed through this inspector, even by a developer account.
func (a *App) devDatabase(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		table = "generation_runs"
	}
	if !devTables[table] {
		problem(w, 400, "TABLE", "Choose an inspectable business table.")
		return
	}
	names := make([]string, 0, len(devTables))
	for name := range devTables {
		names = append(names, name)
	}
	sort.Strings(names)
	cols, err := storage.ReadRows(r.Context(), a.Store.DB, "PRAGMA table_info("+table+")")
	if err != nil {
		problem(w, 500, "DATABASE", "Could not inspect schema.")
		return
	}
	foreignKeys, fkErr := storage.ReadRows(r.Context(), a.Store.DB, "PRAGMA foreign_key_list("+table+")")
	if fkErr != nil {
		problem(w, 500, "DATABASE", "Could not inspect table relationships.")
		return
	}
	var schema string
	_ = a.Store.DB.QueryRowContext(r.Context(), "SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&schema)
	var count int
	_ = a.Store.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if offset > 1000000 {
		offset = 1000000
	}
	entries, err := storage.ReadRows(r.Context(), a.Store.DB, "SELECT * FROM "+table+" ORDER BY id DESC LIMIT 75 OFFSET ?", offset)
	if err != nil {
		problem(w, 500, "DATABASE", "Could not list records.")
		return
	}
	writeJSON(w, 200, map[string]any{"tables": names, "table": table, "columns": cols, "foreign_keys": foreignKeys, "schema": schema, "count": count, "offset": offset, "rows": entries, "editable_fields": domain.Columns(table), "read_only": table == "generation_runs" || table == "scheduled_meetings", "deletable": true})
}
func (a *App) devDatabaseWrite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Table       string         `json:"table"`
		Operation   string         `json:"operation"`
		ID          int            `json:"id"`
		Values      map[string]any `json:"values"`
		Confirm     string         `json:"confirm"`
		Strategy    string         `json:"strategy"`
		Fingerprint string         `json:"fingerprint"`
		Revision    int64          `json:"revision"`
	}
	if err := decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	if !devTables[in.Table] {
		problem(w, 403, "PROTECTED", "This table is not editable in Developer tools.")
		return
	}
	if in.Operation == "delete" {
		if in.ID < 1 {
			problem(w, 400, "ID", "Select a row to delete.")
			return
		}
		strategy := "single"
		confirm := "DELETE"
		if in.Strategy == "preserve" {
			strategy = "preserve"
			confirm = "DELETE AND UNLINK"
		} else if in.Strategy == "linked" {
			strategy = "linked"
			confirm = "DELETE LINKED"
		} else if in.Strategy == "unlink" {
			strategy = "unlink"
			confirm = "UNLINK"
		}
		if in.Confirm != confirm {
			problem(w, 400, "CONFIRM", "Type "+confirm+" to confirm deletion.")
			return
		}
		backup, impact, err := a.applyBusinessDeletion(r.Context(), in.Table, in.ID, strategy, in.Fingerprint, in.Revision, user(r))
		if err != nil {
			problem(w, 409, "DELETE_BLOCKED", safeError(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "deleted": impact.Total, "backup": backup})
		return
	}
	if in.Table == "generation_runs" || in.Table == "scheduled_meetings" {
		problem(w, 403, "IMMUTABLE", "Generated runs and committed meeting times cannot be edited. Select a run to release its reservations and delete it with dependency review.")
		return
	}
	if in.Confirm != "APPLY" {
		problem(w, 400, "CONFIRM", "Type APPLY to edit database records.")
		return
	}
	if in.Operation != "insert" && in.Operation != "update" {
		problem(w, 400, "OPERATION", "Choose insert, update or delete.")
		return
	}
	if in.Operation == "update" && in.ID < 1 {
		problem(w, 400, "ID", "Select a valid record ID.")
		return
	}
	allowed := map[string]bool{}
	for _, field := range domain.Specs[in.Table].Fields {
		allowed[field.Name] = true
	}
	for key := range in.Values {
		if !allowed[key] || key == "id" || key == "created_at" || key == "updated_at" || key == "committed_at" {
			problem(w, 400, "FIELD", "Unknown or protected database column: "+key)
			return
		}
	}
	if len(in.Values) == 0 {
		problem(w, 400, "INPUT", "Specify at least one permitted field.")
		return
	}
	err := a.Store.Write(r.Context(), func(q storage.Querier) error {
		current, _, err := domain.Load(r.Context(), q)
		if err != nil {
			return err
		}
		before := map[string]any{}
		if in.Operation == "update" {
			result, err := storage.ReadRows(r.Context(), q, "SELECT * FROM "+in.Table+" WHERE id=?", in.ID)
			if err != nil {
				return err
			}
			if len(result) == 0 {
				return errors.New("record was removed")
			}
			before = result[0]
		}
		combined := map[string]any{}
		for key, val := range before {
			combined[key] = val
		}
		for key, val := range in.Values {
			combined[key] = val
		}
		if err = domain.ValidateRelations(current, in.Table, combined); err != nil {
			return err
		}
		if err = domain.GuardChanges(current, in.Table, in.ID, before, combined, false); err != nil {
			return err
		}
		var result sql.Result
		if in.Operation == "insert" {
			columns := []string{}
			for k := range in.Values {
				columns = append(columns, k)
			}
			sort.Strings(columns)
			parts := []string{}
			values := []any{}
			for _, k := range columns {
				parts = append(parts, "?")
				values = append(values, in.Values[k])
			}
			result, err = q.ExecContext(r.Context(), "INSERT INTO "+in.Table+" ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(parts, ",")+")", values...)
		} else {
			columns := []string{}
			for k := range in.Values {
				columns = append(columns, k)
			}
			sort.Strings(columns)
			set := []string{}
			values := []any{}
			for _, k := range columns {
				set = append(set, k+"=?")
				values = append(values, in.Values[k])
			}
			values = append(values, in.ID)
			result, err = q.ExecContext(r.Context(), "UPDATE "+in.Table+" SET "+strings.Join(set, ",")+" WHERE id=?", values...)
		}
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return errors.New("record not found or unchanged")
		}
		after, _, err := domain.Load(r.Context(), q)
		if err != nil {
			return err
		}
		if err = validateSnapshot(after); err != nil {
			return err
		}
		for _, assignment := range after.Assignments {
			if !after.Eligible(assignment.FacultyID, assignment.SubjectID) || !after.SectionRequires(assignment.SectionID, assignment.SubjectID) {
				return fmt.Errorf("edit would create an invalid instructor link")
			}
		}
		return audit(r.Context(), q, user(r), "developer-"+in.Operation, in.Table, in.ID, "Direct database edit (generated timestamps are immutable)")
	})
	if err != nil {
		problem(w, 409, "DATABASE_EDIT", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) devSystem(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	var sqliteVer string
	_ = a.Store.DB.QueryRowContext(r.Context(), "SELECT sqlite_version()").Scan(&sqliteVer)
	counts := map[string]int{}
	for name := range devTables {
		var n int
		_ = a.Store.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+name).Scan(&n)
		counts[name] = n
	}
	var integrity string
	_ = a.Store.DB.QueryRowContext(r.Context(), "PRAGMA quick_check").Scan(&integrity)
	machine := readMachine(r.Context(), a.Config.DBPath)
	network := connectivity(r.Context())
	logs, err := storage.ReadRows(r.Context(), a.Store.DB, `SELECT id,action,entity,entity_id,created_at,detail FROM audit_events ORDER BY id DESC LIMIT 40`)
	if err != nil {
		logs = []map[string]any{}
	}
	fileSize := int64(0)
	if info, e := os.Stat(a.Config.DBPath); e == nil {
		fileSize = info.Size()
	}
	writeJSON(w, 200, map[string]any{
		"service": "ASFWMS", "go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"database": "SQLite", "sqlite_version": sqliteVer, "database_integrity": integrity, "db_path": "database/database.db",
		"smtp_configured": a.Config.SMTPEmail != "" && a.Config.SMTPPass != "", "server_time": time.Now().Format(time.RFC3339),
		"heap_mb": mem.Alloc / 1024 / 1024, "go_routines": runtime.NumGoroutine(), "db_bytes": fileSize,
		"machine": machine, "network": network, "logs": logs, "log_source": "Application audit records (not operating-system logs)",
		"counts": counts, "routes": []string{"/api/bootstrap", "/api/generate", "/api/runs/{id}/commit", "/api/timetable/pdf", "/api/access/users", "/api/developer/database"},
		"root": filepath.Base(a.Config.Root),
	})
}
func (a *App) devClearSchedule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DepartmentID  int    `json:"department_id"`
		PeriodID      int    `json:"period_id"`
		Confirm       string `json:"confirm"`
		DeleteHistory bool   `json:"delete_history"`
	}
	if err := decode(r, &in); err != nil || in.Confirm != "CLEAR SCHEDULE" || in.DepartmentID < 1 || in.PeriodID < 1 {
		problem(w, 400, "CONFIRM", "Choose a department and academic period, then type CLEAR SCHEDULE.")
		return
	}
	deleted := int64(0)
	err := a.Store.Write(r.Context(), func(q storage.Querier) error {
		res, err := q.ExecContext(r.Context(), "DELETE FROM scheduled_meetings WHERE department_id=? AND period_id=?", in.DepartmentID, in.PeriodID)
		if err != nil {
			return err
		}
		deleted, _ = res.RowsAffected()
		if _, err = q.ExecContext(r.Context(), "DELETE FROM generation_waits WHERE department_id=? AND period_id=?", in.DepartmentID, in.PeriodID); err != nil {
			return err
		}
		if in.DeleteHistory {
			_, err = q.ExecContext(r.Context(), "DELETE FROM generation_runs WHERE department_id=? AND period_id=?", in.DepartmentID, in.PeriodID)
		} else {
			_, err = q.ExecContext(r.Context(), "UPDATE generation_runs SET status='released' WHERE department_id=? AND period_id=? AND status IN ('committed','preview')", in.DepartmentID, in.PeriodID)
		}
		if err != nil {
			return err
		}
		return audit(r.Context(), q, user(r), "clear-schedule", "generation_runs", in.DepartmentID, fmt.Sprintf("period=%d, meetings=%d, history_removed=%t", in.PeriodID, deleted, in.DeleteHistory))
	})
	if err != nil {
		problem(w, 409, "CLEAR", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deleted_meetings": deleted})
}
