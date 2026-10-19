package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func preview(t *testing.T, h *harness, table string, id int) deletePreview {
	t.Helper()
	_, body := h.call("GET", fmt.Sprintf("/api/records/%s/%d/dependencies", table, id), nil, nil, 200)
	var out deletePreview
	if e := json.Unmarshal(body, &out); e != nil {
		t.Fatal(e)
	}
	if out.Fingerprint == "" {
		t.Fatal("missing fingerprint")
	}
	return out
}
func TestLinkedDeletePeriodProtectsWithoutReviewAndBacksUp(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	run := h.generate(1)
	h.commit(run, 200)
	before := preview(t, h, "academic_periods", 1)
	if before.Related < 3 || !before.CanDelete {
		t.Fatalf("unexpected preview: %+v", before)
	}
	h.call("DELETE", "/api/records/academic_periods/1", map[string]any{"confirm": "DELETE"}, nil, 409)
	if h.count("academic_periods") != 1 || h.count("sections") != 2 {
		t.Fatal("single deletion must not remove children")
	}
	_, body := h.call("DELETE", "/api/records/academic_periods/1", map[string]any{"confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": before.Fingerprint, "revision": before.Revision}, nil, 200)
	var result struct {
		Backup  string `json:"backup"`
		Deleted int    `json:"deleted"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Backup == "" || result.Deleted < 4 {
		t.Fatalf("missing backup / linked removals: %+v", result)
	}
	var seq int
	var main, name string
	if e := h.store.DB.QueryRow("PRAGMA database_list").Scan(&seq, &main, &name); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(name), "backups", result.Backup)); e != nil {
		t.Fatalf("safety backup missing: %v", e)
	}
	for _, table := range []string{"academic_periods", "scheduled_meetings", "generation_runs"} {
		if n := h.count(table); n != 0 {
			t.Fatalf("%s has %d records after linked delete", table, n)
		}
	}
	if h.count("faculty") != 3 || h.count("subjects") != 2 || h.count("curricula") != 2 || h.count("sections") != 2 || h.count("teaching_assignments") != 2 {
		t.Fatal("removing a period should preserve independent and explicit teaching records")
	}
	var orphaned int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM sections WHERE period_id IS NULL").Scan(&orphaned); e != nil || orphaned != 2 {
		t.Fatalf("sections should have unassigned academic period: %d, %v", orphaned, e)
	}
	verifyIntegrity(t, h)
}
func verifyIntegrity(t *testing.T, h *harness) {
	t.Helper()
	var integrity string
	if e := h.store.DB.QueryRow("PRAGMA integrity_check").Scan(&integrity); e != nil || integrity != "ok" {
		t.Fatalf("integrity: %s %v", integrity, e)
	}
	var n int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&n); e != nil || n != 0 {
		t.Fatalf("foreign key check %d %v", n, e)
	}
}
func TestLinkedDeleteFacultyDetachesAdviserButPreservesSection(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "faculty", 3)
	if len(p.Detaches) != 1 || p.Detaches[0].Table != "sections" || p.Detaches[0].Count != 1 {
		t.Fatalf("expected one adviser detachment: %+v", p.Detaches)
	}
	h.call("DELETE", "/api/records/faculty/3", map[string]any{"confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	var adviser any
	if e := h.store.DB.QueryRow("SELECT adviser_id FROM sections WHERE id=1").Scan(&adviser); e != nil {
		t.Fatal(e)
	}
	if adviser != nil {
		t.Fatalf("adviser should be detached, got %v", adviser)
	}
	if h.count("sections") != 2 {
		t.Fatal("section should have been preserved")
	}
	verifyIntegrity(t, h)
}
func TestCurriculumLinkedDeleteKeepsOtherDepartment(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "curricula", 1)
	h.call("DELETE", "/api/records/curricula/1", map[string]any{"confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("sections") != 2 || h.count("curricula") != 1 || h.count("teaching_assignments") != 1 || h.count("subjects") != 2 {
		t.Fatal("independent sections and subjects must survive; invalid link rows should be removed")
	}
	var detached int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM sections WHERE curriculum_id IS NULL").Scan(&detached); e != nil || detached != 1 {
		t.Fatalf("dependent section did not become unassigned: %d, %v", detached, e)
	}
	verifyIntegrity(t, h)
}
func TestCurriculumMovePreservesSectionsAndAssignments(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, e := h.store.DB.Exec(`INSERT INTO curricula(id,program_id,name) VALUES(3,1,'Replacement');INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(3,3,1,1)`)
	if e != nil {
		t.Fatal(e)
	}
	p := preview(t, h, "curricula", 1)
	h.call("POST", "/api/records/curricula/1/move-dependencies", map[string]any{"target_id": 3, "confirm": "MOVE AND DELETE", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("sections") != 2 || h.count("teaching_assignments") != 2 {
		t.Fatal("sections or assignments lost")
	}
	var cid int
	_ = h.store.DB.QueryRow("SELECT curriculum_id FROM sections WHERE id=1").Scan(&cid)
	if cid != 3 {
		t.Fatalf("section not transferred: %d", cid)
	}
	verifyIntegrity(t, h)
}
func TestPeriodMovePreservesSections(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, e := h.store.DB.Exec(`INSERT INTO academic_periods(id,label,academic_year,semester) VALUES(2,'New period','2027-2028','First')`)
	if e != nil {
		t.Fatal(e)
	}
	p := preview(t, h, "academic_periods", 1)
	_, body := h.call("POST", "/api/records/academic_periods/1/move-dependencies", map[string]any{"target_id": 2, "confirm": "MOVE AND DELETE", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if !strings.Contains(string(body), "sections_moved") {
		t.Fatal("did not report moved sections")
	}
	if h.count("sections") != 2 || h.count("academic_periods") != 1 {
		t.Fatal("period move removed sections")
	}
	verifyIntegrity(t, h)
}
func TestCurriculumMoveBlockedIfSubjectsMissing(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, e := h.store.DB.Exec(`INSERT INTO curricula(id,program_id,name) VALUES(3,1,'Empty replacement')`)
	if e != nil {
		t.Fatal(e)
	}
	p := preview(t, h, "curricula", 1)
	h.call("POST", "/api/records/curricula/1/move-dependencies", map[string]any{"target_id": 3, "confirm": "MOVE AND DELETE", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	if h.count("curricula") != 3 || h.count("sections") != 2 {
		t.Fatal("invalid move mutated records")
	}
}
func TestDeveloperGeneratedRunCanBeDeletedButCreatedAtCannotBeEdited(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	run := h.generate(1)
	h.commit(run, 200)
	p := preview(t, h, "generation_runs", run)
	h.call("POST", "/api/developer/database", map[string]any{"table": "generation_runs", "id": run, "operation": "update", "confirm": "APPLY", "values": map[string]any{"created_at": "2000-01-01"}}, nil, 403)
	h.call("POST", "/api/developer/database", map[string]any{"table": "generation_runs", "id": run, "operation": "delete", "confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("scheduled_meetings") != 0 || h.count("generation_runs") != 0 {
		t.Fatal("run not fully removed")
	}
	verifyIntegrity(t, h)
}
func TestDeleteRejectsStalePreview(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "curricula", 1)
	_, e := h.store.DB.Exec("UPDATE curricula SET name='Changed' WHERE id=1")
	if e != nil {
		t.Fatal(e)
	}
	h.call("DELETE", "/api/records/curricula/1", map[string]any{"confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	if h.count("curricula") != 2 {
		t.Fatal("stale delete removed a record")
	}
}
func TestGeneratedScheduleDeletionDoesNotInvalidateUnrelatedDepartment(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	a := h.generate(1)
	h.commit(a, 200)
	b := h.generate(2)
	h.commit(b, 200)
	p := preview(t, h, "generation_runs", a)
	h.call("POST", "/api/developer/database", map[string]any{"table": "generation_runs", "operation": "delete", "id": a, "values": map[string]any{}, "confirm": "DELETE LINKED", "strategy": "linked", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("generation_runs") != 1 {
		t.Fatal("unrelated run removed")
	}
	var remaining int
	if err := h.store.DB.QueryRow("SELECT count(*) FROM scheduled_meetings WHERE run_id=?", b).Scan(&remaining); err != nil || remaining == 0 {
		t.Fatal("unrelated meetings removed")
	}
}

func TestDepartmentDeletionDoesNotTouchAccounts(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/access/users", map[string]any{
		"name": "Department Scheduler", "username": "department_user", "email": "department@example.test",
		"password": "Department-test-password-42", "role": "scheduler", "department_id": 1,
	}, nil, 201)
	p := preview(t, h, "departments", 1)
	if !p.Blocked || p.CanDelete {
		t.Fatalf("account linked department must be protected: %+v", p)
	}
	h.call("DELETE", "/api/records/departments/1", map[string]any{"strategy": "linked", "confirm": "DELETE LINKED", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	if h.count("departments") != 2 || h.count("users") != 2 {
		t.Fatal("department/account should survive")
	}
}
func TestEditingGeneratedTimestampsIsRejected(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/developer/database", map[string]any{"table": "faculty", "operation": "update", "id": 1, "confirm": "APPLY", "values": map[string]any{"created_at": "2000-01-01"}}, nil, 400)
	var before string
	if e := h.store.DB.QueryRow("SELECT name FROM faculty WHERE id=1").Scan(&before); e != nil || before != "Instructor One" {
		t.Fatal("timestamp tamper modified faculty")
	}
}

// Schedulers may inspect owned records without receiving cross-department details.
func TestScopedSchedulerDeletionPreviewDoesNotLeakDependencies(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/access/users", map[string]any{
		"name": "Dept Scheduler", "username": "preview_scope", "email": "preview@example.test",
		"password": "Long-password-preview-test-42", "role": "scheduler", "department_id": 1,
	}, nil, 201)
	staff := newRoleSession(t, h, "preview_scope", "Long-password-preview-test-42")
	staff.call("GET", "/api/records/faculty/2/dependencies", nil, 403)
	_, body := staff.call("GET", "/api/records/faculty/1/dependencies", nil, 200)
	var result deletePreview
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Related > 0 && (!result.Blocked || result.CanDelete || len(result.Impacts) > 0 || len(result.Detaches) > 0) {
		t.Fatalf("scheduler preview leaked linked record details: %+v", result)
	}
}

func TestCurriculumMoveAlsoRemovesStaleJSONOnlyPreviewRuns(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	run := h.generate(1) // Preview only, no scheduled_meetings yet.
	_, err := h.store.DB.Exec(`INSERT INTO curricula(id,program_id,name) VALUES(3,1,'Compatible replacement');INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(3,3,1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	p := preview(t, h, "curricula", 1)
	included := false
	for _, item := range p.Impacts {
		if item.Table == "generation_runs" {
			included = true
		}
	}
	if !included {
		t.Fatalf("uncommitted run #%d was not included in curriculum impact", run)
	}
	h.call("POST", "/api/records/curricula/1/move-dependencies", map[string]any{"target_id": 3, "confirm": "MOVE AND DELETE", "revision": p.Revision, "fingerprint": p.Fingerprint}, nil, 200)
	var count int
	if err := h.store.DB.QueryRow("SELECT COUNT(*) FROM generation_runs WHERE id=?", run).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old preview run #%d survives moving its section: %v", run, err)
	}
	verifyIntegrity(t, h)
}

// Removing a curriculum member is a relationship update, not subject deletion.
func TestUnlinkCurriculumMembershipPreservesOriginalRecords(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, err := h.store.DB.Exec("INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(3,1,1,2)")
	if err != nil {
		t.Fatal(err)
	}
	p := preview(t, h, "curriculum_subject_years", 3)
	if p.Related != 0 {
		t.Fatalf("new standalone link unexpectedly has dependencies: %+v", p)
	}
	h.call("DELETE", "/api/records/curriculum_subject_years/3", map[string]any{"strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("curriculum_subject_years") != 2 || h.count("subjects") != 2 || h.count("curricula") != 2 || h.count("faculty") != 3 || h.count("sections") != 2 {
		t.Fatal("unlink removed unrelated original records")
	}
	verifyIntegrity(t, h)
}
func TestUnlinkAssignedRelationshipRefusesCascade(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	for _, item := range []struct {
		table string
		id    int
	}{
		{"curriculum_subject_years", 1}, {"faculty_subjects", 1},
	} {
		p := preview(t, h, item.table, item.id)
		if p.Related == 0 {
			t.Fatalf("expected linked assignments on %s", item.table)
		}
		url := fmt.Sprintf("/api/records/%s/%d", item.table, item.id)
		h.call("DELETE", url, map[string]any{"strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	}
	if h.count("curriculum_subject_years") != 2 || h.count("faculty_subjects") != 3 || h.count("teaching_assignments") != 2 {
		t.Fatal("protected links were removed")
	}
	verifyIntegrity(t, h)
}
func TestUnlinkExplicitTeachingAssignmentPreservesFacultyEligibility(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "teaching_assignments", 1)
	if p.Related != 0 {
		t.Fatalf("no timetable exists yet: %+v", p)
	}
	h.call("DELETE", "/api/records/teaching_assignments/1", map[string]any{"strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("teaching_assignments") != 1 || h.count("faculty_subjects") != 3 || h.count("subjects") != 2 || h.count("sections") != 2 {
		t.Fatal("unlink affected subject eligibility or underlying records")
	}
	verifyIntegrity(t, h)
}
func TestUnlinkCannotDeletePrimarySubjectOrUseStalePreview(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "subjects", 1)
	h.call("DELETE", "/api/records/subjects/1", map[string]any{"strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	p = preview(t, h, "faculty_subjects", 3)
	_, err := h.store.DB.Exec("UPDATE faculty SET name='Updated name' WHERE id=3")
	if err != nil {
		t.Fatal(err)
	}
	h.call("DELETE", "/api/records/faculty_subjects/3", map[string]any{"strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 409)
	if h.count("subjects") != 2 || h.count("faculty_subjects") != 3 {
		t.Fatal("unlink bypassed protections")
	}
	verifyIntegrity(t, h)
}
func TestDeveloperDatabaseUnlinkKeepsSubject(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "faculty_subjects", 3)
	h.call("POST", "/api/developer/database", map[string]any{"table": "faculty_subjects", "id": 3, "operation": "delete", "strategy": "unlink", "confirm": "UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("faculty_subjects") != 2 || h.count("subjects") != 2 || h.count("faculty") != 3 {
		t.Fatal("developer unlink removed parent records")
	}
	verifyIntegrity(t, h)
}

func TestUnlinkAdviserKeepsSectionAndFaculty(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("PUT", "/api/records/sections/1", map[string]any{"adviser_id": nil}, nil, 200)
	var adviser any
	if err := h.store.DB.QueryRow("SELECT adviser_id FROM sections WHERE id=1").Scan(&adviser); err != nil || adviser != nil {
		t.Fatalf("adviser link not removed: %v %v", adviser, err)
	}
	if h.count("sections") != 2 || h.count("faculty") != 3 || h.count("teaching_assignments") != 2 {
		t.Fatal("adviser unlink should preserve original records and teaching assignments")
	}
	verifyIntegrity(t, h)
}

func TestCurriculumUnlinkPreviewListsImpactedSections(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "curriculum_subject_years", 1)
	if len(p.UnlinkEffects) != 1 || p.UnlinkEffects[0].Table != "sections" || p.UnlinkEffects[0].Count != 1 || p.UnlinkEffects[0].Samples[0].Label == "" {
		t.Fatalf("unlinked subject needs a section-impact preview: %+v", p.UnlinkEffects)
	}
}
