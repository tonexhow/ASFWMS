package httpapi

import (
	"asfwms.local/scheduler/internal/scheduling"
	"testing"
)

func TestSectionAutoCurriculumAndOptionalAdviser(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	// Two semester-scoped curricula, one per term. Existing legacy remains usable.
	h.call("POST", "/api/records/curricula", map[string]any{"program_id": 1, "name": "Y1 S1", "year_level": 1, "semester": "1st Semester", "active": 1}, nil, 200)
	h.call("POST", "/api/records/curricula", map[string]any{"program_id": 1, "name": "Y1 S2", "year_level": 1, "semester": "2nd Semester", "active": 1}, nil, 200)
	h.call("POST", "/api/records/sections", map[string]any{"program_id": 1, "code": "IT-NO-ADVISER", "year_level": 1, "size": 40, "active": 1}, nil, 200)
	var id, adviser, period int
	if err := h.store.DB.QueryRow("SELECT coalesce(curriculum_id,0),coalesce(adviser_id,0),coalesce(period_id,0) FROM sections WHERE code='IT-NO-ADVISER'").Scan(&id, &adviser, &period); err != nil || id != 0 || adviser != 0 || period != 0 {
		t.Fatalf("expected term-neutral bank section without adviser, curriculum=%d adviser=%d period=%d err=%v", id, adviser, period, err)
	}
	// Changing the academic term never rewrites the section bank.
	if _, err := h.store.DB.Exec("UPDATE academic_periods SET semester='2nd Semester' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	h.call("PUT", "/api/records/sections/3", map[string]any{"year_level": 1}, nil, 200)
	if err := h.store.DB.QueryRow("SELECT coalesce(curriculum_id,0), coalesce(period_id,0) FROM sections WHERE code='IT-NO-ADVISER'").Scan(&id, &period); err != nil || id != 0 || period != 0 {
		t.Fatalf("bank became term-dependent: %d, %d, %v", id, period, err)
	}
	h.call("PUT", "/api/records/sections/3", map[string]any{"adviser_id": nil}, nil, 200)
	// Missing curriculum is a generation-preflight issue, not a reason to reject a reusable section.
	h.call("POST", "/api/records/sections", map[string]any{"program_id": 1, "code": "IT-UNMATCHED", "year_level": 2}, nil, 200)
	// Run a readiness check: missing adviser is a warning, not a placement error.
	_, data := h.call("POST", "/api/readiness", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "standard"}, nil, 200)
	if len(data) == 0 {
		t.Fatal("empty readiness response")
	}
}
