package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"errors"
	"reflect"
	"testing"
)

func fixture() domain.Snapshot {
	return domain.Snapshot{
		Departments: []domain.Department{{ID: 1, Code: "CITE", Active: 1}, {ID: 2, Code: "COTE", Active: 1}},
		Programs:    []domain.Program{{ID: 1, DepartmentID: 1, Active: 1}, {ID: 2, DepartmentID: 2, Active: 1}},
		Periods:     []domain.Period{{ID: 1, Label: "Test", Semester: "1st Semester", StartDate: "2026-08-01", EndDate: "2026-12-31", Active: 1, DayStart: 420, DayEnd: 1140, LunchStart: 660, LunchEnd: 840}},
		Curricula:   []domain.Curriculum{{ID: 1, ProgramID: 1, Active: 1}, {ID: 2, ProgramID: 2, Active: 1}},
		Subjects:    []domain.Subject{{ID: 1, Code: "MAJOR", Title: "Major", WeeklyMinutes: 180, Priority: "major", Pattern: "mwf", RequiredRoom: "any", AllowedDays: "weekdays", Active: 1}},
		Mappings:    []domain.Mapping{{ID: 1, CurriculumID: 1, SubjectID: 1, Year: 1}},
		Sections:    []domain.Section{{ID: 1, Code: "IT-1A", ProgramID: 1, CurriculumID: 1, PeriodID: 1, Year: 1, Size: 40, AdviserID: 2, Active: 1}},
		Faculty:     []domain.Faculty{{ID: 1, Name: "Instructor", Active: 1}, {ID: 2, Name: "Adviser only", Active: 1}},
		Eligibility: []domain.Eligibility{{ID: 1, FacultyID: 1, SubjectID: 1}},
		Assignments: []domain.Assignment{{ID: 1, FacultyID: 1, SubjectID: 1, SectionID: 1}},
		Buildings:   []domain.Building{{ID: 1, Active: 1}},
		Rooms:       []domain.Room{{ID: 1, BuildingID: 1, DepartmentID: 1, Code: "Own", Kind: "classroom", Capacity: 50, Start: 420, End: 1140, Active: 1}},
	}
}
func request() Request { return Request{DepartmentID: 1, PeriodID: 1, Mode: "standard", Seed: 42} }
func TestCompleteAndAdviserIndependence(t *testing.T) {
	s := fixture()
	r, e := Generate(context.Background(), s, request())
	if e != nil {
		t.Fatal(e)
	}
	if r.Outcome != "complete" || r.PlacedMinutes != 180 || len(r.Meetings) != 3 {
		t.Fatalf("unexpected result %+v", r)
	}
	for _, m := range r.Meetings {
		if m.FacultyID != 1 {
			t.Fatal("adviser became instructor")
		}
	}
	if e = Validate(s, r); e != nil {
		t.Fatal(e)
	}
}
func TestAdviserDoesNotFillMissingInstructor(t *testing.T) {
	s := fixture()
	s.Assignments = nil
	s.Eligibility = nil
	r, e := Generate(context.Background(), s, request())
	if e != nil || r.Placed != 0 || r.Issues[0].Code != "MISSING_INSTRUCTOR" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestCurriculumUsesSelectedYear(t *testing.T) {
	s := fixture()
	s.Mappings = append(s.Mappings, domain.Mapping{ID: 2, CurriculumID: 1, SubjectID: 1, Year: 2})
	r, _ := Generate(context.Background(), s, request())
	if r.Required != 1 {
		t.Fatal("other year leaked into requirements")
	}
}
func TestPartialRetainsFeasibleClasses(t *testing.T) {
	s := fixture()
	other := s.Subjects[0]
	other.ID = 2
	other.Code = "MISSING"
	s.Subjects = append(s.Subjects, other)
	s.Mappings = append(s.Mappings, domain.Mapping{ID: 2, CurriculumID: 1, SubjectID: 2, Year: 1})
	r, e := Generate(context.Background(), s, request())
	if e != nil || r.Outcome != "partial" || r.Required != 2 || r.Placed != 1 || r.PlacedMinutes != 180 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestForceBorrowsButStandardDoesNot(t *testing.T) {
	s := fixture()
	s.Rooms[0].DepartmentID = 2
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 0 || r.Issues[0].Code != "LACK_OF_FACILITY" {
		t.Fatal(r)
	}
	req := request()
	req.Mode = "force"
	r, e := Generate(context.Background(), s, req)
	if e != nil || r.Placed != 1 || r.Borrowed != 3 {
		t.Fatalf("%+v %v", r, e)
	}
	if e = Validate(s, r); e != nil {
		t.Fatal(e)
	}
}
func TestOwnRoomPreferredOverEarlierSharedRoom(t *testing.T) {
	s := fixture()
	s.Rooms[0].Start = 840
	s.Rooms = append(s.Rooms, domain.Room{ID: 2, BuildingID: 1, Code: "Shared", DepartmentID: 0, Kind: "classroom", Capacity: 50, Start: 420, End: 1140, Active: 1})
	r, _ := Generate(context.Background(), s, request())
	for _, m := range r.Meetings {
		if m.RoomID != 1 {
			t.Fatal("did not prioritize department room")
		}
	}
}
func TestMajorFirstWhenOnlyOneClassFits(t *testing.T) {
	s := fixture()
	s.Subjects[0].Pattern = "single"
	minor := s.Subjects[0]
	minor.ID = 2
	minor.Code = "MINOR"
	minor.Priority = "minor"
	s.Subjects = append([]domain.Subject{minor}, s.Subjects...)
	s.Mappings = append([]domain.Mapping{{ID: 2, CurriculumID: 1, SubjectID: 2, Year: 1}}, s.Mappings...)
	s.Eligibility = append(s.Eligibility, domain.Eligibility{FacultyID: 1, SubjectID: 2})
	s.Assignments = append(s.Assignments, domain.Assignment{FacultyID: 1, SubjectID: 2, SectionID: 1})
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 1, Start: 420, End: 600}}
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 1 || r.Meetings[0].SubjectID != 1 {
		t.Fatalf("major not prioritized: %+v", r)
	}
}
func TestNoPartialCourseMeetingsAreKept(t *testing.T) {
	s := fixture()
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 1, Start: 420, End: 1140}}
	r, _ := Generate(context.Background(), s, request())
	if len(r.Meetings) != 0 || r.Placed != 0 {
		t.Fatal("kept an incomplete MWF course")
	}
}
func TestForceRespectsCommittedReservations(t *testing.T) {
	s := fixture()
	s.Rooms[0].DepartmentID = 2
	s.Meetings = []domain.Meeting{{DepartmentID: 2, PeriodID: 1, SectionID: 9, SubjectID: 9, FacultyID: 2, RoomID: 1, Day: 1, Start: 420, End: 600}}
	req := request()
	req.Mode = "force"
	r, _ := Generate(context.Background(), s, req)
	for _, m := range r.Meetings {
		for _, x := range s.Meetings {
			if overlaps(m, x) && m.RoomID == x.RoomID {
				t.Fatal("double booking")
			}
		}
	}
	if len(s.Meetings) != 1 {
		t.Fatal("mutated existing schedule")
	}
	if e := Validate(s, r); e != nil {
		t.Fatal(e)
	}
}
func TestFacultyAvailabilityReplacesDefault(t *testing.T) {
	s := fixture()
	s.Subjects[0].Pattern = "tth"
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 2, Start: 540, End: 720}, {FacultyID: 1, Day: 4, Start: 540, End: 720}}
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 1 {
		t.Fatal(r)
	}
	for _, m := range r.Meetings {
		if m.Start < 540 || m.End > 720 || (m.Day != 2 && m.Day != 4) {
			t.Fatal(m)
		}
	}
}
func TestWorkloadLimit(t *testing.T) {
	s := fixture()
	s.Faculty[0].MaxWeeklyMinutes = 120
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 0 {
		t.Fatal("exceeded workload")
	}
}
func TestLunchAndConsecutiveRules(t *testing.T) {
	s := fixture()
	s.Periods[0].LunchMinutes = 60
	s.Periods[0].MaxConsecutive = 3
	e, _ := prepare(s, request())
	q := Requirement{Section: s.Sections[0], Subject: s.Subjects[0], Faculty: s.Faculty[0], Explicit: true}
	m := e.meeting(q, s.Rooms[0], part{1, 180}, 660)
	if why := e.check(q, []domain.Meeting{m}, nil); why != "LUNCH_BREAK" {
		t.Fatal(why)
	}
	s.Periods[0].LunchMinutes = 0
	e, _ = prepare(s, request())
	base := []domain.Meeting{}
	for i := 0; i < 3; i++ {
		base = append(base, e.meeting(q, s.Rooms[0], part{1, 60}, 420+i*60))
	}
	m = e.meeting(q, s.Rooms[0], part{1, 60}, 600)
	if why := e.check(q, []domain.Meeting{m}, base); why != "SECTION_CONSECUTIVE_LIMIT" {
		t.Fatal(why)
	}
	m.Start = 660
	m.End = 720
	if why := e.check(q, []domain.Meeting{m}, base); why != "" {
		t.Fatal(why)
	}
}
func TestRoomsDoNotNeedLunch(t *testing.T) {
	s := fixture()
	s.Periods[0].LunchMinutes = 60
	e, _ := prepare(s, request())
	q := Requirement{Section: s.Sections[0], Subject: s.Subjects[0], Faculty: s.Faculty[0], Explicit: true}
	base := []domain.Meeting{{DepartmentID: 2, PeriodID: 1, RoomID: 1, FacultyID: 2, SectionID: 2, Day: 1, Start: 660, End: 720}}
	m := e.meeting(q, s.Rooms[0], part{1, 60}, 720)
	if why := e.check(q, []domain.Meeting{m}, base); why != "" {
		t.Fatal("room incorrectly required a break:", why)
	}
}
func TestOverlappingPeriodsShareResources(t *testing.T) {
	s := fixture()
	other := s.Periods[0]
	other.ID = 2
	s.Periods = append(s.Periods, other)
	s.Meetings = []domain.Meeting{{DepartmentID: 2, PeriodID: 2, RoomID: 1, FacultyID: 2, SectionID: 2, Day: 1, Start: 420, End: 600}}
	r, _ := Generate(context.Background(), s, request())
	for _, m := range r.Meetings {
		if overlaps(m, s.Meetings[0]) {
			t.Fatal("overlapping period collision")
		}
	}
	s.Periods[1].StartDate = "2027-01-01"
	s.Periods[1].EndDate = "2027-05-01"
	e, _ := prepare(s, request())
	if len(e.base) != 0 {
		t.Fatal("disjoint periods unnecessarily reserve time")
	}
}
func TestDeterministicAndCancellation(t *testing.T) {
	s := fixture()
	a, _ := Generate(context.Background(), s, request())
	b, _ := Generate(context.Background(), s, request())
	if !reflect.DeepEqual(a.Meetings, b.Meetings) {
		t.Fatal("same seed not reproducible")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := Generate(ctx, s, request())
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestValidationRejectsChangedInstructor(t *testing.T) {
	s := fixture()
	r, _ := Generate(context.Background(), s, request())
	r.Meetings[0].FacultyID = 2
	if e := Validate(s, r); e == nil {
		t.Fatal("accepted different instructor")
	}
}
func TestSaturdayNeedsSubjectAndFacultyPermission(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "saturday"
	s.Subjects[0].Pattern = "sat"
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 0 {
		t.Fatal("Saturday was enabled silently")
	}
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 6, Start: 420, End: 1140}}
	r, _ = Generate(context.Background(), s, request())
	if r.Placed != 1 || r.Meetings[0].Day != 6 {
		t.Fatal(r)
	}
}

func TestAutomaticFacultyWhenSectionNotExplicitlyAssigned(t *testing.T) {
	s := fixture()
	s.Assignments = nil
	result, err := Generate(context.Background(), s, request())
	if err != nil || result.Placed != 1 || result.Outcome != "complete" {
		t.Fatalf("auto assignment failed: %+v %v", result, err)
	}
	for _, meeting := range result.Meetings {
		if meeting.FacultyID != 1 {
			t.Fatalf("adviser incorrectly chosen as instructor: %+v", meeting)
		}
	}
	if err := Validate(s, result); err != nil {
		t.Fatalf("auto commit must be valid: %v", err)
	}
	if len(s.Assignments) != 0 {
		t.Fatal("automatic choice mutated explicit assignment")
	}
}
func TestAutomaticFacultyFallsBackWhenFirstUnavailable(t *testing.T) {
	s := fixture()
	s.Assignments = nil
	s.Faculty = append(s.Faculty, domain.Faculty{ID: 3, Name: "Another", DepartmentID: 1, Active: 1})
	s.Eligibility = append(s.Eligibility, domain.Eligibility{ID: 3, FacultyID: 3, SubjectID: 1})
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 1, Start: 420, End: 600}, {FacultyID: 3, Day: 1, Start: 420, End: 1140}, {FacultyID: 3, Day: 3, Start: 420, End: 1140}, {FacultyID: 3, Day: 5, Start: 420, End: 1140}}
	result, err := Generate(context.Background(), s, request())
	if err != nil || result.Placed != 1 {
		t.Fatalf("fallback failed: %+v %v", result, err)
	}
	for _, m := range result.Meetings {
		if m.FacultyID != 3 {
			t.Fatalf("must choose feasible faculty #3, got %d", m.FacultyID)
		}
	}
	if err := Validate(s, result); err != nil {
		t.Fatal(err)
	}
}
func TestExplicitInstructorNotSubstituted(t *testing.T) {
	s := fixture()
	s.Faculty = append(s.Faculty, domain.Faculty{ID: 3, Name: "Another", Active: 1})
	s.Eligibility = append(s.Eligibility, domain.Eligibility{FacultyID: 3, SubjectID: 1})
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 1, Start: 420, End: 600}, {FacultyID: 3, Day: 1, Start: 420, End: 1140}, {FacultyID: 3, Day: 3, Start: 420, End: 1140}, {FacultyID: 3, Day: 5, Start: 420, End: 1140}}
	result, err := Generate(context.Background(), s, request())
	if err != nil || result.Placed != 0 {
		t.Fatalf("explicit assignment was silently substituted: %+v %v", result, err)
	}
}

func TestMissingAdviserWarnsWithoutBlockingGeneration(t *testing.T) {
	s := fixture()
	s.Sections[0].AdviserID = 0
	pre, err := Preflight(s, request())
	if err != nil || len(pre.Warnings) != 1 || pre.Warnings[0].Code != "MISSING_ADVISER" || len(pre.Issues) != 0 {
		t.Fatalf("preflight %+v %v", pre, err)
	}
	result, err := Generate(context.Background(), s, request())
	if err != nil || result.Outcome != "complete" || result.Placed != 1 || len(result.Issues) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("generation %+v %v", result, err)
	}
	if err := Validate(s, result); err != nil {
		t.Fatalf("warnings wrongly blocked validation: %v", err)
	}
}
