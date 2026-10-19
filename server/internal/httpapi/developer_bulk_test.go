package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type testBulkPreview struct {
	SelectedCount int    `json:"selected_count"`
	ConfirmPhrase string `json:"confirm_phrase"`
	Impact        struct {
		Revision    int64  `json:"revision"`
		Fingerprint string `json:"fingerprint"`
		Total       int    `json:"total"`
		Related     int    `json:"related"`
		Blocked     bool   `json:"blocked"`
	} `json:"impact"`
}

func previewBulk(t *testing.T, h *harness, table, scope string, ids []int) testBulkPreview {
	t.Helper()
	_, b := h.call("POST", "/api/developer/database/bulk-preview", map[string]any{"table": table, "scope": scope, "ids": ids}, nil, 200)
	var preview testBulkPreview
	if e := json.Unmarshal(b, &preview); e != nil {
		t.Fatal(e)
	}
	if preview.Impact.Fingerprint == "" || preview.ConfirmPhrase == "" {
		t.Fatalf("preview missing confirmation: %s", b)
	}
	return preview
}

func bulkBody(table, scope string, ids []int, prev testBulkPreview) map[string]any {
	return map[string]any{
		"table": table, "scope": scope, "ids": ids,
		"expected_count": prev.SelectedCount,
		"fingerprint":    prev.Impact.Fingerprint,
		"revision":       prev.Impact.Revision,
		"confirm":        prev.ConfirmPhrase,
	}
}

func TestDeveloperBulkDeleteSelectedSectionsPreservesIndependentRows(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	prev := previewBulk(t, h, "sections", "selected", []int{1, 2})
	if prev.SelectedCount != 2 || prev.Impact.Total < 4 || prev.Impact.Blocked {
		t.Fatalf("unexpected preview: %#v", prev)
	}
	bad := bulkBody("sections", "selected", []int{1, 2}, prev)
	bad["confirm"] = "DELETE"
	h.call("POST", "/api/developer/database/bulk-delete", bad, nil, 400)
	if h.count("sections") != 2 {
		t.Fatal("unconfirmed delete affected sections")
	}
	_, b := h.call("POST", "/api/developer/database/bulk-delete", bulkBody("sections", "selected", []int{1, 2}, prev), nil, 200)
	var outcome struct {
		Deleted int    `json:"selected_deleted"`
		Backup  string `json:"backup"`
	}
	if e := json.Unmarshal(b, &outcome); e != nil {
		t.Fatal(e)
	}
	if outcome.Deleted != 2 || outcome.Backup == "" {
		t.Fatalf("missing bulk backup/response: %s", b)
	}
	var databasePath string
	if e := h.store.DB.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&databasePath); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(databasePath), "backups", outcome.Backup)); e != nil {
		t.Fatalf("safety backup not present: %v", e)
	}
	for table, count := range map[string]int{"sections": 0, "teaching_assignments": 0, "subjects": 2, "faculty": 3, "curricula": 2, "programs": 2, "faculty_subjects": 3} {
		if got := h.count(table); got != count {
			t.Fatalf("%s: got %d want %d", table, got, count)
		}
	}
	if h.count("users") != 1 {
		t.Fatal("protected developer account deleted")
	}
	var badKeys int
	if err := h.store.DB.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&badKeys); err != nil || badKeys != 0 {
		t.Fatalf("foreign key violations: %d, %v", badKeys, err)
	}
}

func TestDeveloperBulkDeleteAllAcrossPages(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	for i := 3; i <= 82; i++ {
		if _, e := h.store.DB.Exec("INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level,size) VALUES(?,1,1,1,?,1,25)", i, fmt.Sprintf("CITE-%d", i)); e != nil {
			t.Fatal(e)
		}
	}
	prev := previewBulk(t, h, "sections", "all", nil)
	if prev.SelectedCount != 82 || prev.ConfirmPhrase != "DELETE ALL SECTIONS" {
		t.Fatalf("select-all missing rows beyond first 75: %#v", prev)
	}
	h.call("POST", "/api/developer/database/bulk-delete", bulkBody("sections", "all", nil, prev), nil, 200)
	if h.count("sections") != 0 || h.count("subjects") != 2 || h.count("curricula") != 2 {
		t.Fatal("bulk all removed the wrong records")
	}
}

func TestDeveloperBulkDeleteRejectsStaleSelectionAndProtectedTables(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/developer/database/bulk-preview", map[string]any{"table": "users", "scope": "all"}, nil, 400)
	h.call("POST", "/api/developer/database/bulk-delete", map[string]any{"table": "sessions", "scope": "all"}, nil, 400)
	h.call("POST", "/api/developer/database/bulk-preview", map[string]any{"table": "sections", "scope": "selected", "ids": []int{1, 1}}, nil, 400)
	prev := previewBulk(t, h, "sections", "all", nil)
	if _, e := h.store.DB.Exec("INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level,size) VALUES(3,1,1,1,'NEW-SECTION',1,20)"); e != nil {
		t.Fatal(e)
	}
	h.call("POST", "/api/developer/database/bulk-delete", bulkBody("sections", "all", nil, prev), nil, 400)
	if h.count("sections") != 3 {
		t.Fatal("stale all-table preview deleted data")
	}
	selected := previewBulk(t, h, "sections", "selected", []int{1})
	body := bulkBody("sections", "selected", []int{1}, selected)
	if _, e := h.store.DB.Exec("DELETE FROM sections WHERE id=3"); e != nil {
		t.Fatal(e)
	}
	// A selected-only preview must not silently turn into a select-all operation.
	body["scope"] = "all"
	h.call("POST", "/api/developer/database/bulk-delete", body, nil, 400)
	if h.count("sections") != 2 {
		t.Fatal("changed-scope request modified sections")
	}
}

func TestDeveloperBulkDeleteCurriculumLeavesSectionsAndMasterSubjects(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := previewBulk(t, h, "curricula", "selected", []int{1})
	if p.SelectedCount != 1 || p.Impact.Blocked {
		t.Fatalf("unexpected curriculum preview: %+v", p)
	}
	h.call("POST", "/api/developer/database/bulk-delete", bulkBody("curricula", "selected", []int{1}, p), nil, 200)
	if h.count("sections") != 2 || h.count("subjects") != 2 || h.count("curricula") != 1 {
		t.Fatal("independent records were removed while deleting curriculum")
	}
	var cID *int
	if e := h.store.DB.QueryRow("SELECT curriculum_id FROM sections WHERE id=1").Scan(&cID); e != nil || cID != nil {
		t.Fatalf("section curriculum link should be null: %v / %v", cID, e)
	}
}

func TestDeveloperBulkDeleteReleasesAffectedCommittedRun(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	runID := h.generate(1)
	h.commit(runID, 200)
	if h.count("scheduled_meetings") == 0 {
		t.Fatal("fixture did not commit any meetings")
	}
	prev := previewBulk(t, h, "sections", "selected", []int{1})
	if prev.Impact.Total <= prev.SelectedCount {
		t.Fatal("committed timetable dependencies missing from preview")
	}
	h.call("POST", "/api/developer/database/bulk-delete", bulkBody("sections", "selected", []int{1}, prev), nil, 200)
	if h.count("sections") != 1 || h.count("generation_runs") != 0 || h.count("scheduled_meetings") != 0 {
		t.Fatal("deleting a scheduled section did not release the whole saved run")
	}
	if h.count("subjects") != 2 || h.count("faculty") != 3 {
		t.Fatal("bulk section removal removed faculty or subject masters")
	}
}

func TestDeveloperBulkDeleteEndpointsDeveloperOnly(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/access/users", map[string]any{"name": "Administrator", "email": "admin-bulk@example.org", "username": "bulk_admin", "password": "StrongBulkPassword-42", "role": "administrator"}, nil, 201)
	admin := newRoleSession(t, h, "bulk_admin", "StrongBulkPassword-42")
	request := map[string]any{"table": "sections", "scope": "all"}
	admin.call("POST", "/api/developer/database/bulk-preview", request, 403)
	admin.call("POST", "/api/developer/database/bulk-delete", request, 403)
	if h.count("sections") != 2 {
		t.Fatal("non-developer changed records")
	}
}
