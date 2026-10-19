package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"testing"
)

func TestPhysicalFacilityAliasesCannotDoubleBook(t *testing.T) {
	s := fixture()
	s.Rooms[0].DepartmentID = 0
	s.Rooms[0].Name = "IT Lab 1"
	s.Rooms[0].Kind = "laboratory"
	s.Rooms[0].Code = "CITE-1"
	canonical := s.Rooms[0]
	canonical.ID = 2
	canonical.DepartmentID = 1
	canonical.Code = "ITLAB1"
	canonical.Name = "IT Laboratory 1"
	s.Rooms = append(s.Rooms, canonical)
	e, err := prepare(s, request())
	if err != nil {
		t.Fatal(err)
	}
	q := Requirement{Section: s.Sections[0], Subject: s.Subjects[0], Faculty: s.Faculty[0], Explicit: true}
	opts := e.candidateRooms(q)
	if len(opts) != 1 || opts[0].ID != 2 {
		t.Fatalf("expected one canonical owned lab, got %+v", opts)
	}
	candidate := e.meeting(q, canonical, part{day: 1, duration: 60}, 540)
	// Another faculty and section already occupy the old alias at the same
	// physical location, but not at the same room ID.
	busy := []domain.Meeting{{PeriodID: 1, SectionID: 99, FacultyID: 99, RoomID: 1, Day: 1, Start: 540, End: 600}}
	if got := e.check(q, []domain.Meeting{candidate}, busy); got != "FACILITY_CONFLICT" {
		t.Fatalf("alias overlap permitted: %q", got)
	}
}
