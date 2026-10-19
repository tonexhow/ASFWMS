package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"testing"
)

func TestSundaySubjectNeedsExplicitFacultySundayAvailability(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "7"
	s.Subjects[0].Pattern = "auto"
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 0 {
		t.Fatalf("Sunday placed outside faculty week: %+v %v", r, err)
	}
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 7, Start: 480, End: 660}}
	r, err = Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || len(r.Meetings) != 1 || r.Meetings[0].Day != 7 || r.Meetings[0].Start != 480 {
		t.Fatalf("Sunday-only placement: %+v %v", r, err)
	}
	if err = Validate(s, r); err != nil {
		t.Fatal(err)
	}
	r.Meetings[0].Day = 1
	if err = Validate(s, r); err == nil {
		t.Fatal("commit accepted a non-selected day")
	}
}
func TestSaturdayFallbackSearchesAllPrimaryRoomsFirst(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "1"
	s.Subjects[0].Pattern = "single"
	s.Subjects[0].SaturdayFallback = 1
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 1, Start: 420, End: 600}, {FacultyID: 1, Day: 6, Start: 420, End: 600}}
	s.Meetings = []domain.Meeting{{DepartmentID: 2, PeriodID: 1, SectionID: 99, FacultyID: 99, RoomID: 1, Day: 1, Start: 420, End: 1140}}
	s.Rooms = append(s.Rooms, domain.Room{ID: 2, BuildingID: 1, DepartmentID: 0, Active: 1, Kind: "classroom", Capacity: 50, Start: 420, End: 1140})
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || r.Meetings[0].Day != 1 || r.Meetings[0].RoomID != 2 {
		t.Fatalf("chose Saturday while a primary-day room was free: %+v %v", r, err)
	}
	s.Rooms = s.Rooms[:1]
	r, err = Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || r.Meetings[0].Day != 6 {
		t.Fatalf("fallback not used: %+v %v", r, err)
	}
	if err = Validate(s, r); err != nil {
		t.Fatal(err)
	}
	s.Subjects[0].SaturdayFallback = 0
	r, err = Generate(context.Background(), s, request())
	if err != nil || r.Placed != 0 {
		t.Fatalf("Saturday fallback enabled without permission: %+v %v", r, err)
	}
}
func TestSelectedWeekdaySubsetAndExactMinutes(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "2,5"
	s.Subjects[0].Pattern = "auto"
	s.Subjects[0].WeeklyMinutes = 300
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || r.PlacedMinutes != 300 {
		t.Fatalf("subset failed: %+v %v", r, err)
	}
	for _, m := range r.Meetings {
		if m.Day != 2 && m.Day != 5 {
			t.Fatal("non-selected day", m)
		}
	}
	if err = Validate(s, r); err != nil {
		t.Fatal(err)
	}
}
func TestMajorsPrecedeConsideredMinorsAndNeverBypassHardLimits(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "1"
	s.Subjects[0].Pattern = "single"
	s.Subjects[0].WeeklyMinutes = 60
	s.Periods[0].DayEnd = 480
	s.Rooms[0].End = 480
	minor := s.Subjects[0]
	minor.ID = 2
	minor.Priority = "minor"
	minor.Code = "MINOR"
	s.Subjects = append(s.Subjects, minor)
	s.Mappings = append(s.Mappings, domain.Mapping{ID: 2, CurriculumID: 1, SubjectID: 2, Year: 1})
	s.Faculty[1].PriorityScheduling = 1
	s.Eligibility = append(s.Eligibility, domain.Eligibility{ID: 2, FacultyID: 2, SubjectID: 2})
	s.Assignments = append(s.Assignments, domain.Assignment{ID: 2, FacultyID: 2, SubjectID: 2, SectionID: 1})
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || r.Meetings[0].SubjectID != 1 {
		t.Fatalf("major must precede considered minor: %+v %v", r, err)
	}
	// With both instructors considered, the major must lead within their group.
	s.Faculty[0].PriorityScheduling = 1
	r, _ = Generate(context.Background(), s, request())
	if r.Placed != 1 || r.Meetings[0].SubjectID != 1 {
		t.Fatalf("major within priority group lost: %+v", r)
	}
	// A priority flag never creates availability outside the working window.
	s.Faculty[0].PriorityScheduling = 0
	s.WorkTimes = []domain.Window{{FacultyID: 2, Day: 2, Start: 420, End: 480}}
	r, _ = Generate(context.Background(), s, request())
	if r.Placed != 1 || r.Meetings[0].SubjectID != 1 {
		t.Fatalf("priority bypassed availability: %+v", r)
	}
}
func TestMandatorySaturdayIsNotAFallback(t *testing.T) {
	s := fixture()
	s.Subjects[0].AllowedDays = "6"
	s.Subjects[0].Pattern = "auto"
	s.Subjects[0].SaturdayFallback = 0
	s.WorkTimes = []domain.Window{{FacultyID: 1, Day: 6, Start: 420, End: 1140}}
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 || r.Meetings[0].Day != 6 {
		t.Fatalf("mandatory Saturday failed: %+v %v", r, err)
	}
}
