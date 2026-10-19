package httpapi

import (
	"asfwms.local/scheduler/internal/scheduling"
	"context"
	"encoding/json"
	"testing"
)

func TestFacultySubjectSelectionIsAtomicAndProtectsAssignments(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/faculty/1/subjects", map[string]any{"subject_ids": []int{1, 2}}, nil, 200)
	if h.count("faculty_subjects") != 4 {
		t.Fatal("eligible subject not added")
	}
	h.call("POST", "/api/faculty/1/subjects", map[string]any{"subject_ids": []int{2}}, nil, 409)
	if h.count("faculty_subjects") != 4 {
		t.Fatal("locked subject removal partly changed data")
	}
	h.call("POST", "/api/faculty/1/subjects", map[string]any{"subject_ids": []int{1, 999999}}, nil, 409)
	if h.count("faculty_subjects") != 4 {
		t.Fatal("invalid selection partly changed data")
	}
	h.call("POST", "/api/faculty/1/subjects", map[string]any{"subject_ids": []int{1, 1}}, nil, 200)
	if h.count("faculty_subjects") != 3 {
		t.Fatal("selection diff not applied")
	}
	// Matching eligibility alone does not make a subject part of a section's year.
	h.call("POST", "/api/assignments", map[string]any{"faculty_id": 2, "subject_id": 2, "section_ids": []int{1}}, nil, 409)
	if h.count("teaching_assignments") != 2 {
		t.Fatal("incompatible section assigned")
	}
}
func TestCurriculumMembersAndNewContainers(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/records/curricula", map[string]any{"program_id": 1, "name": "Another curriculum"}, nil, 200)
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 2, "subject_ids": []int{1, 2}}, nil, 200)
	if h.count("curricula") != 3 || h.count("curriculum_subject_years") != 4 {
		t.Fatal("wrong container/member count")
	}
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 2, "subject_ids": []int{1, 2}}, nil, 200)
	if h.count("curriculum_subject_years") != 4 {
		t.Fatal("duplicate members")
	}
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 3, "subject_ids": []int{1, 999999}}, nil, 409)
	if h.count("curriculum_subject_years") != 4 {
		t.Fatal("invalid batch partially committed")
	}
	h.call("POST", "/api/curricula/1/members", map[string]any{"year_level": 0, "subject_ids": []int{1}}, nil, 400)
}
func TestSubjectSundayAndFacultyPriorityAPI(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("PUT", "/api/records/subjects/1", map[string]any{"allowed_days": "7", "pattern": "sun", "saturday_fallback": 0}, nil, 200)
	h.call("PUT", "/api/records/faculty/1", map[string]any{"priority_scheduling": 1}, nil, 200)
	h.call("POST", "/api/faculty/1/work-times", map[string]any{"days": []map[string]int{{"day": 7, "start_minute": 480, "end_minute": 660}}}, nil, 200)
	_, data := h.call("POST", "/api/generate", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "standard", Seed: 42}, nil, 201)
	var run struct {
		ID     int               `json:"id"`
		Result scheduling.Result `json:"result"`
	}
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Result.Placed != 1 || run.Result.Meetings[0].Day != 7 {
		t.Fatalf("Sunday not generated: %s", data)
	}
	h.commit(run.ID, 200)
	// Priority can change without rewriting existing reservations.
	h.call("PUT", "/api/records/faculty/1", map[string]any{"priority_scheduling": 0}, nil, 200)
	if h.count("scheduled_meetings") != 1 {
		t.Fatal("priority toggle changed reservations")
	}
	// There is intentionally no academic-period weekend override.
	h.call("PUT", "/api/records/academic_periods/1", map[string]any{"allow_saturday": 1}, nil, 409)
}

func TestSourceDatasetGeneratesIndependentConflictFreeDepartments(t *testing.T) {
	h := newHarness(t)
	if err := h.store.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	summary := []map[string]any{}
	for _, dept := range []int{23, 21, 22, 20} {
		t.Logf("Generating department %d", dept)
		_, body := h.call("POST", "/api/generate", scheduling.Request{DepartmentID: dept, PeriodID: 8, Mode: "force", ConfirmBorrow: "FORCE", Seed: 42}, nil, 201)
		var run struct {
			ID     int               `json:"id"`
			Result scheduling.Result `json:"result"`
		}
		if err := json.Unmarshal(body, &run); err != nil {
			t.Fatal(err)
		}
		if len(run.Result.Meetings) > 0 {
			h.commit(run.ID, 200)
		}
		summary = append(summary, map[string]any{"department": dept, "required": run.Result.Required, "placed": run.Result.Placed, "meetings": len(run.Result.Meetings), "minutes": run.Result.PlacedMinutes, "issues": len(run.Result.Issues), "borrowed": run.Result.Borrowed})
	}
	var overlaps, mismatches int
	err := h.store.DB.QueryRow(`SELECT count(*) FROM scheduled_meetings a JOIN scheduled_meetings b ON a.id<b.id AND a.day=b.day AND a.start_minute<b.end_minute AND b.start_minute<a.end_minute AND (a.faculty_id=b.faculty_id OR a.room_id=b.room_id OR a.section_id=b.section_id)`).Scan(&overlaps)
	if err != nil {
		t.Fatal(err)
	}
	err = h.store.DB.QueryRow(`SELECT count(*) FROM (SELECT m.section_id,m.subject_id,SUM(m.end_minute-m.start_minute) actual,s.weekly_minutes expected FROM scheduled_meetings m JOIN subjects s ON s.id=m.subject_id GROUP BY m.section_id,m.subject_id HAVING actual<>expected)`).Scan(&mismatches)
	if err != nil {
		t.Fatal(err)
	}
	if overlaps != 0 || mismatches != 0 {
		t.Fatalf("overlaps=%d, duration mismatches=%d", overlaps, mismatches)
	}
	t.Logf("Source dataset (department 23=COTE,21=CITE,22=COHM,20=CCJE): %+v; committed meetings=%d; overlaps=0; contact mismatches=0", summary, h.count("scheduled_meetings"))
}

func TestSubjectFacultySelectionPreservesExplicitAndCommittedLinks(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	// Selecting instructors from Academic > Subjects edits eligibility only.
	h.call("POST", "/api/subjects/2/faculty", map[string]any{"faculty_ids": []int{1, 2}}, nil, 200)
	h.call("POST", "/api/subjects/1/faculty", map[string]any{"faculty_ids": []int{2}}, nil, 409) // faculty #1 explicitly teaches it
	h.call("POST", "/api/subjects/2/faculty", map[string]any{"faculty_ids": []int{2, 999999}}, nil, 409)
	var count int
	if err := h.store.DB.QueryRow("SELECT count(*) FROM faculty_subjects WHERE subject_id=2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("invalid write was not atomic, count=%d", count)
	}
	h.call("POST", "/api/subjects/2/faculty", map[string]any{"faculty_ids": []int{2}}, nil, 200)
	if err := h.store.DB.QueryRow("SELECT count(*) FROM faculty_subjects WHERE subject_id=2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("eligible instructors not adjusted, count=%d", count)
	}
}
