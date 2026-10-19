package httpapi

import (
	"asfwms.local/scheduler/internal/scheduling"
	"testing"
)

func TestExplicitInstructorReassignmentRequiresEligibilityAndProtectsCommitted(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/assignments/reassign", map[string]any{
		"faculty_id": 2, "subject_id": 1, "section_id": 1,
	}, nil, 400) // deliberate double-action confirmation required
	h.call("POST", "/api/assignments/reassign", map[string]any{
		"faculty_id": 3, "subject_id": 1, "section_id": 1, "confirm": "REASSIGN",
	}, nil, 409) // adviser alone does not create subject eligibility
	if h.count("teaching_assignments") != 2 {
		t.Fatal("failed change altered assignments")
	}
	h.call("POST", "/api/assignments/reassign", map[string]any{
		"faculty_id": 2, "subject_id": 1, "section_id": 1, "confirm": "REASSIGN",
	}, nil, 200)
	var assigned int
	if err := h.store.DB.QueryRow("SELECT faculty_id FROM teaching_assignments WHERE section_id=1 AND subject_id=1").Scan(&assigned); err != nil || assigned != 2 {
		t.Fatalf("reassignment failed; faculty=%d, error=%v", assigned, err)
	}
	h.call("POST", "/api/assignments/reassign", map[string]any{
		"faculty_id": 2, "subject_id": 1, "section_id": 1, "confirm": "REASSIGN",
	}, nil, 200) // idempotent
	h.call("POST", "/api/generate", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "standard", Seed: 42}, nil, 201)
	run := h.generate(1)
	h.commit(run, 200)
	h.call("POST", "/api/assignments/reassign", map[string]any{
		"faculty_id": 1, "subject_id": 1, "section_id": 1, "confirm": "REASSIGN",
	}, nil, 409) // committed meeting must be deliberately released first
}

func TestCurriculumMembershipPreventsDuplicateProgramYearOwners(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	if _, err := h.store.DB.Exec(`INSERT INTO curricula(id,program_id,name) VALUES(3,1,'Parallel legacy owner');
        INSERT INTO curriculum_subject_years(curriculum_id,subject_id,year_level) VALUES(3,1,2);`); err != nil {
		t.Fatal(err)
	}
	count := h.count("curriculum_subject_years")
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 2, "subject_ids": []int{1}}, nil, 409)
	if got := h.count("curriculum_subject_years"); got != count {
		t.Fatalf("duplicate-year write changed memberships: %d vs %d", got, count)
	}
	// Other academic years remain editable in the original curriculum.
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 3, "subject_ids": []int{1}}, nil, 200)
}
