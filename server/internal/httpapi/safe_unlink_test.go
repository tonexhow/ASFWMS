package httpapi

import (
	"strconv"
	"testing"
)

// Independent records are retained. Rows representing only an association or
// an invalidated generated timetable can be removed, and the parent FK becomes NULL.
func TestCurriculumDeletionPreservesSectionAndAllowsRelink(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	originalSubjects, originalSections := h.count("subjects"), h.count("sections")
	before := preview(t, h, "curricula", 1)
	foundDetach := false
	for _, group := range before.Detaches {
		if group.Table == "sections" && group.Field == "curriculum_id" && group.Count == 1 {
			foundDetach = true
		}
	}
	if !foundDetach {
		t.Fatalf("sections should be detached rather than deleted: %+v", before)
	}
	h.call("DELETE", "/api/records/curricula/1", map[string]any{"strategy": "preserve", "confirm": "DELETE AND UNLINK", "fingerprint": before.Fingerprint, "revision": before.Revision}, nil, 200)
	if h.count("subjects") != originalSubjects || h.count("sections") != originalSections || h.count("curricula") != 1 {
		t.Fatal("deleted independent rows")
	}
	var c any
	if e := h.store.DB.QueryRow("SELECT curriculum_id FROM sections WHERE id=1").Scan(&c); e != nil || c != nil {
		t.Fatalf("curriculum column not cleared: %v %v", c, e)
	}
	if h.count("teaching_assignments") != 1 {
		t.Fatal("invalid instructor link not cleared")
	}
	// A new curriculum reuses the EXISTING subject, without duplicating a subject row.
	h.call("POST", "/api/records/curricula", map[string]any{"program_id": 1, "name": "Replacement curriculum"}, nil, 200)
	var newID int
	if e := h.store.DB.QueryRow("SELECT id FROM curricula WHERE name='Replacement curriculum'").Scan(&newID); e != nil {
		t.Fatal(e)
	}
	h.call("POST", "/api/curricula/"+itoa(newID)+"/members", map[string]any{"year_level": 1, "subject_ids": []int{1}}, nil, 200)
	h.call("PUT", "/api/records/sections/1", map[string]any{"curriculum_id": newID}, nil, 200)
	if h.count("subjects") != originalSubjects || h.count("sections") != originalSections {
		t.Fatal("relink duplicated/deleted independent data")
	}
	verifyIntegrity(t, h)
}

func TestPeriodDeletionRetainsInstructorRelationships(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "academic_periods", 1)
	h.call("DELETE", "/api/records/academic_periods/1", map[string]any{"strategy": "preserve", "confirm": "DELETE AND UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("sections") != 2 || h.count("teaching_assignments") != 2 {
		t.Fatal("period deletion removed sections or their teacher links")
	}
	var cleared int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM sections WHERE period_id IS NULL").Scan(&cleared); e != nil || cleared != 2 {
		t.Fatalf("period not detached: %d %v", cleared, e)
	}
	verifyIntegrity(t, h)
}

func TestBuildingDeletionPreservesRoomAndReassignsIt(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "buildings", 1)
	if len(p.Detaches) == 0 {
		t.Fatalf("room linkage should be cleared: %+v", p)
	}
	h.call("DELETE", "/api/records/buildings/1", map[string]any{"strategy": "preserve", "confirm": "DELETE AND UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("rooms") != 2 || h.count("buildings") != 0 {
		t.Fatal("rooms should survive building deletion")
	}
	var cleared int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM rooms WHERE building_id IS NULL").Scan(&cleared); e != nil || cleared != 2 {
		t.Fatalf("rooms not detached: %d %v", cleared, e)
	}
	verifyIntegrity(t, h)
}

func TestProgramDeletionKeepsSectionAndCurriculum(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	p := preview(t, h, "programs", 1)
	h.call("DELETE", "/api/records/programs/1", map[string]any{"strategy": "preserve", "confirm": "DELETE AND UNLINK", "fingerprint": p.Fingerprint, "revision": p.Revision}, nil, 200)
	if h.count("sections") != 2 || h.count("curricula") != 2 || h.count("subjects") != 2 || h.count("programs") != 1 {
		t.Fatal("program deletion should not delete independent linked entities")
	}
	var emptyCurr, emptySection int
	_ = h.store.DB.QueryRow("SELECT count(*) FROM curricula WHERE program_id IS NULL").Scan(&emptyCurr)
	_ = h.store.DB.QueryRow("SELECT count(*) FROM sections WHERE program_id IS NULL").Scan(&emptySection)
	if emptyCurr != 1 || emptySection != 1 {
		t.Fatalf("orphan state not recorded: curriculum=%d section=%d", emptyCurr, emptySection)
	}
	verifyIntegrity(t, h)
}

func itoa(n int) string { return strconv.Itoa(n) }
