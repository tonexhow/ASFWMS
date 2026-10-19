package httpapi

import (
	"testing"
)

func TestScopedCurriculumCreateThenAddSubjects(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	beforeSubjects := h.count("subjects")
	beforeLegacyLinks := h.count("curriculum_subject_years")
	h.call("POST", "/api/records/curricula", map[string]any{
		"program_id": 1, "name": "BSIT / Year 1 / 1st Semester",
		"year_level": 1, "semester": "1st Semester", "active": 1,
	}, nil, 200)
	// Duplicate active year/semester is blocked even before any subjects are mapped.
	h.call("POST", "/api/records/curricula", map[string]any{
		"program_id": 1, "name": "Duplicate scoped curriculum",
		"year_level": 1, "semester": "1st Semester", "active": 1,
	}, nil, 409)
	// Other semesters may have independent curricula for the same program/year.
	h.call("POST", "/api/records/curricula", map[string]any{
		"program_id": 1, "name": "BSIT / Year 1 / 2nd Semester",
		"year_level": 1, "semester": "2nd Semester", "active": 1,
	}, nil, 200)
	h.call("POST", "/api/curricula/3/members", map[string]any{
		"year_level": 2, "subject_ids": []int{1},
	}, nil, 409)
	h.call("POST", "/api/curricula/3/members", map[string]any{
		"year_level": 1, "subject_ids": []int{1, 2},
	}, nil, 200)
	if h.count("subjects") != beforeSubjects {
		t.Fatal("linking copied or deleted original subjects")
	}
	if h.count("curriculum_subject_years") != beforeLegacyLinks+2 {
		t.Fatal("missing mapped subjects or changed old membership")
	}
	var mappedSem string
	if err := h.store.DB.QueryRow("SELECT source_semester FROM curriculum_subject_years WHERE curriculum_id=3 AND subject_id=1").Scan(&mappedSem); err != nil || mappedSem != "1st Semester" {
		t.Fatalf("subject link did not persist selected semester: %q %v", mappedSem, err)
	}
	h.call("POST", "/api/records/curriculum_subject_years", map[string]any{
		"curriculum_id": 4, "subject_id": 1, "year_level": 1, "source_semester": "1st Semester",
	}, nil, 409)
	// A section's academic period must match the scoped semester.
	if _, err := h.store.DB.Exec("UPDATE academic_periods SET semester='2nd Semester' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	h.call("POST", "/api/records/sections", map[string]any{
		"program_id": 1, "curriculum_id": 3, "period_id": 1, "code": "IT-NEW", "year_level": 1,
		"size": 30, "active": 1,
	}, nil, 409)
	if _, err := h.store.DB.Exec("UPDATE academic_periods SET semester='1st Semester' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	h.call("POST", "/api/records/sections", map[string]any{
		"program_id": 1, "curriculum_id": 3, "period_id": 1, "code": "IT-NEW", "year_level": 1,
		"size": 30, "active": 1,
	}, nil, 200)
	// The second-semester curriculum may not be attached to a first-semester section.
	h.call("POST", "/api/records/sections", map[string]any{
		"program_id": 1, "curriculum_id": 4, "period_id": 1, "code": "IT-WRONG-SEM", "year_level": 1,
		"size": 30, "active": 1,
	}, nil, 409)
	if h.count("subjects") != beforeSubjects {
		t.Fatal("original subject count changed")
	}
}
