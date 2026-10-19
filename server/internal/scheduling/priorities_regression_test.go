package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"testing"
)

func TestFacultyLunchIsRequiredEvenWhenPeriodLunchIsOptional(t *testing.T) {
	s := fixture()
	s.Periods[0].LunchMinutes = 0
	e, err := prepare(s, request())
	if err != nil {
		t.Fatal(err)
	}
	q := Requirement{Faculty: s.Faculty[0], Section: s.Sections[0], Subject: s.Subjects[0], Explicit: true}
	// 11:00-14:00 is entirely booked: no 60-minute faculty lunch survives.
	proposed := []domain.Meeting{e.meeting(q, s.Rooms[0], part{day: 1, duration: 180}, 660)}
	if why := e.check(q, proposed, nil); why != "LUNCH_BREAK" {
		t.Fatalf("expected faculty lunch rejection, got %q", why)
	}
	// A meeting ending at 12:00 still leaves a valid 60-minute interval.
	proposed[0] = e.meeting(q, s.Rooms[0], part{day: 1, duration: 60}, 660)
	if why := e.check(q, proposed, nil); why != "" {
		t.Fatalf("rejected available lunch interval: %s", why)
	}
}

func TestFacultyCanTeachFourBackToBackWhenSectionsDiffer(t *testing.T) {
	s := fixture()
	s.Periods[0].LunchMinutes = 0
	e, err := prepare(s, request())
	if err != nil {
		t.Fatal(err)
	}
	q := Requirement{Faculty: s.Faculty[0], Section: s.Sections[0], Subject: s.Subjects[0], Explicit: true}
	busy := []domain.Meeting{}
	for i := 0; i < 3; i++ {
		m := e.meeting(q, s.Rooms[0], part{day: 1, duration: 60}, 420+60*i)
		m.SectionID = 100 + i // other sections, same instructor
		busy = append(busy, m)
	}
	m := e.meeting(q, s.Rooms[0], part{day: 1, duration: 60}, 600)
	if why := e.check(q, []domain.Meeting{m}, busy); why != "" {
		t.Fatalf("faculty consecutive classes were incorrectly blocked: %s", why)
	}
	// The same four meetings for a single section DO violate the limit.
	for i := range busy {
		busy[i].SectionID = q.Section.ID
		busy[i].FacultyID = 100 + i
	}
	if why := e.check(q, []domain.Meeting{m}, busy); why != "SECTION_CONSECUTIVE_LIMIT" {
		t.Fatalf("section exceeded consecutive limit without rejection: %s", why)
	}
}

func TestMajorCoreHoursAndMinorEdgeHoursArePreferences(t *testing.T) {
	s := fixture()
	e, err := prepare(s, request())
	if err != nil {
		t.Fatal(err)
	}
	q := Requirement{Faculty: s.Faculty[0], Section: s.Sections[0], Subject: s.Subjects[0]}
	q.Subject.Priority = "major"
	major := e.candidateTimes(q, 60, nil)
	if len(major) == 0 || major[0] != 480 {
		t.Fatalf("major didn't prefer 08:00: %v", major)
	}
	q.Subject.Priority = "minor"
	minor := e.candidateTimes(q, 60, nil)
	if len(minor) == 0 || minor[0] != 420 {
		t.Fatalf("minor didn't prefer 07:00 edge: %v", minor)
	}
}
