package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

// Bulk changes are deliberately available only under /api/developer/ routes.
// The middleware enforces a live Developer session and CSRF protection.
// Only ordinary business tables on the explicit devTables allowlist are eligible.
type bulkDatabaseRequest struct {
	Table         string `json:"table"`
	Scope         string `json:"scope"` // selected or all, for this one table only
	IDs           []int  `json:"ids"`
	ExpectedCount int    `json:"expected_count"`
	Fingerprint   string `json:"fingerprint"`
	Revision      int64  `json:"revision"`
	Confirm       string `json:"confirm"`
}

// The caller must validate the table name against devTables before any SQL is built.
func resolveBulkIDs(ctx context.Context, q storage.Querier, table, scope string, ids []int) ([]int, error) {
	if !devTables[table] {
		return nil, fmt.Errorf("only inspectable business tables can be bulk deleted")
	}
	if scope != "selected" && scope != "all" {
		return nil, fmt.Errorf("choose selected rows or all rows in one table")
	}
	if scope == "all" {
		if len(ids) != 0 {
			return nil, fmt.Errorf("an all-table operation cannot specify individual IDs")
		}
		// One extra ID detects the limit without loading an unlimited table.
		all, err := queryIDs(ctx, q, "SELECT id FROM "+table+" ORDER BY id LIMIT 5001")
		if err != nil {
			return nil, err
		}
		if len(all) > 5000 {
			return nil, fmt.Errorf("this table has over 5000 records; delete smaller selections first")
		}
		return all, nil
	}
	if len(ids) == 0 || len(ids) > 5000 {
		return nil, fmt.Errorf("select between 1 and 5000 rows")
	}
	found := map[int]bool{}
	for _, id := range ids {
		if id < 1 || found[id] {
			return nil, fmt.Errorf("selection contains invalid or repeated IDs")
		}
		found[id] = true
	}
	selected := append([]int(nil), ids...)
	sort.Ints(selected)
	return selected, nil
}

func bulkPhrase(table, scope string, count int) string {
	if scope == "all" {
		return "DELETE ALL " + strings.ToUpper(table)
	}
	return fmt.Sprintf("DELETE %d %s", count, strings.ToUpper(table))
}

func (a *App) devBulkPreview(w http.ResponseWriter, r *http.Request) {
	if user(r).Role != "developer" {
		deny(w)
		return
	}
	var in bulkDatabaseRequest
	if err := decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	ids, err := resolveBulkIDs(r.Context(), a.Store.DB, in.Table, in.Scope, in.IDs)
	if err != nil || len(ids) == 0 {
		if err == nil {
			err = fmt.Errorf("there are no records to delete")
		}
		problem(w, 400, "SELECTION", err.Error())
		return
	}
	plan, err := buildDeletePlanForIDs(r.Context(), a.Store.DB, in.Table, ids)
	if err != nil {
		problem(w, 409, "DEPENDENCIES", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{
		"table": in.Table, "scope": in.Scope, "selected_count": len(ids),
		"confirm_phrase": bulkPhrase(in.Table, in.Scope, len(ids)),
		"impact":         plan.Preview,
	})
}

func (a *App) devBulkDelete(w http.ResponseWriter, r *http.Request) {
	if user(r).Role != "developer" {
		deny(w)
		return
	}
	var in bulkDatabaseRequest
	if err := decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	ids, err := resolveBulkIDs(r.Context(), a.Store.DB, in.Table, in.Scope, in.IDs)
	if err != nil || len(ids) == 0 {
		problem(w, 400, "SELECTION", "Invalid or empty table selection. Refresh and try again.")
		return
	}
	if in.ExpectedCount != len(ids) || in.Confirm != bulkPhrase(in.Table, in.Scope, len(ids)) || in.Fingerprint == "" {
		problem(w, 400, "CONFIRM", "Selection or confirmation does not match the reviewed preview.")
		return
	}
	plan, err := buildDeletePlanForIDs(r.Context(), a.Store.DB, in.Table, ids)
	if err != nil {
		problem(w, 409, "DEPENDENCIES", safeError(err))
		return
	}
	if plan.Preview.Blocked {
		problem(w, 409, "PROTECTED", "This selection includes links to protected account data. Resolve those links first.")
		return
	}
	if in.Revision != plan.Preview.Revision || in.Fingerprint != plan.Preview.Fingerprint {
		problem(w, 409, "STALE", "Records or dependencies have changed. Preview the selection again.")
		return
	}
	// Even unlinked/standalone bulk deletion requires a safety backup.
	backupPath, err := a.linkedDeleteBackup(r.Context())
	if err != nil {
		problem(w, 500, "BACKUP", "Could not create a safety backup: "+safeError(err))
		return
	}
	var completed int
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		currentIDs, e := resolveBulkIDs(r.Context(), q, in.Table, in.Scope, in.IDs)
		if e != nil || len(currentIDs) != len(ids) {
			return fmt.Errorf("selection changed; review the impact again")
		}
		current, e := buildDeletePlanForIDs(r.Context(), q, in.Table, currentIDs)
		if e != nil {
			return e
		}
		if current.Preview.Blocked || current.Preview.Revision != in.Revision || current.Preview.Fingerprint != in.Fingerprint {
			return fmt.Errorf("dependencies changed; review the impact again")
		}
		if e = executeDeletePlan(r.Context(), q, current); e != nil {
			return e
		}
		snapshot, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		if e = validateSnapshot(snapshot); e != nil {
			return e
		}
		completed = len(currentIDs)
		return audit(r.Context(), q, user(r), "developer-bulk-delete", in.Table, 0,
			fmt.Sprintf("scope=%s roots=%d affected=%d related=%d backup=%s", in.Scope, len(currentIDs), current.Preview.Total, current.Preview.Related, filepath.Base(backupPath)))
	})
	if err != nil {
		problem(w, 409, "DELETE_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "selected_deleted": completed, "affected_deleted": plan.Preview.Total, "backup": filepath.Base(backupPath)})
}
