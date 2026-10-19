package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"strings"
	"testing"
)

// A stable section ID must be reusable across semesters. Its cached legacy
// period/curriculum links must never override the generation request.
func TestSectionBankResolvesCurriculumForSelectedSemester(t *testing.T) {
	s := fixture()
	s.Programs[0].Code = "BSIT"
	s.Sections[0].PeriodID = 0
	s.Sections[0].CurriculumID = 0
	s.Sections[0].AdviserID = 0
	s.Curricula[0].YearLevel = 1
	s.Curricula[0].Semester = "1st Semester"
	s.Curricula = append(s.Curricula, domain.Curriculum{ID: 3, ProgramID: 1, YearLevel: 1, Semester: "2nd Semester", Active: 1})
	s.Periods = append(s.Periods, domain.Period{ID: 2, Label: "Second term", Semester: "2nd Semester", StartDate: "2027-01-01", EndDate: "2027-05-01", Active: 1, DayStart: 420, DayEnd: 1140, LunchStart: 660, LunchEnd: 840})
	second := s.Subjects[0]
	second.ID = 3
	second.Code = "TERM-2"
	second.Title = "Second Term Subject"
	s.Subjects = append(s.Subjects, second)
	s.Mappings = append(s.Mappings, domain.Mapping{ID: 3, CurriculumID: 3, SubjectID: 3, Year: 1})

	first, e := Preflight(s, Request{DepartmentID: 1, PeriodID: 1, Mode: "standard"})
	if e != nil || first.Required != 1 {
		t.Fatalf("first term requirement: %+v, %v", first, e)
	}
	secondTerm, e := Preflight(s, Request{DepartmentID: 1, PeriodID: 2, Mode: "standard"})
	if e != nil || secondTerm.Required != 1 {
		t.Fatalf("second term requirement: %+v, %v", secondTerm, e)
	}
	found := false
	for _, issue := range secondTerm.Issues {
		if issue.Code == "MISSING_INSTRUCTOR" && issue.Subject == "TERM-2" && issue.SubjectTitle == "Second Term Subject" && issue.Section == "IT-1A" {
			found = true
		}
	}
	if !found {
		t.Fatalf("preflight did not name the missing second-term instructor precisely: %+v", secondTerm.Issues)
	}
	for _, issue := range first.Issues {
		if issue.Subject == "TERM-2" {
			t.Fatalf("second-term subject leaked into first term: %+v", issue)
		}
	}
	if len(first.Warnings) != 1 || !strings.Contains(first.Warnings[0].Code, "ADVISER") {
		t.Fatalf("missing adviser not a separate optional reminder: %+v", first.Warnings)
	}
	// Add eligibility; the exact same Section row can then be scheduled in term 2.
	s.Eligibility = append(s.Eligibility, domain.Eligibility{ID: 3, FacultyID: 1, SubjectID: 3})
	r, err := Generate(context.Background(), s, Request{DepartmentID: 1, PeriodID: 2, Mode: "standard", Seed: 42})
	if err != nil || r.Outcome != "complete" || r.Placed != 1 || len(r.Meetings) == 0 {
		t.Fatalf("second-term schedule: %+v, %v", r, err)
	}
	for _, m := range r.Meetings {
		if m.SubjectID != 3 || m.SectionID != 1 || m.PeriodID != 2 {
			t.Fatalf("wrong term mapping: %+v", m)
		}
	}
}
