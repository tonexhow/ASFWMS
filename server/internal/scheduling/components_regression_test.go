package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"reflect"
	"testing"
)

func splitFixture() domain.Snapshot {
	s := fixture()
	s.Subjects[0].Units = 3
	s.Subjects[0].RequiredRoom = "any"
	s.Components = []domain.SubjectComponent{{ID: 11, SubjectID: 1, Kind: "lecture", Units: 1, WeeklyMinutes: 60}, {ID: 12, SubjectID: 1, Kind: "laboratory", Units: 2, WeeklyMinutes: 120}}
	s.Rooms = append(s.Rooms, domain.Room{ID: 2, BuildingID: 1, DepartmentID: 1, Code: "Lab", Kind: "laboratory", Capacity: 50, Start: 420, End: 1140, Active: 1})
	return s
}
func TestSplitComponentsPlaceExactMinutesAndDifferentDays(t *testing.T) {
	s := splitFixture()
	r, err := Generate(context.Background(), s, request())
	if err != nil || r.Placed != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	mins := map[string]int{}
	used := map[int]string{}
	for _, m := range r.Meetings {
		mins[m.ComponentKind] += m.End - m.Start
		if m.FacultyID != 1 {
			t.Fatal("components assigned to different faculty")
		}
		if m.ComponentKind == "lecture" && m.RoomID != 1 || m.ComponentKind == "laboratory" && m.RoomID != 2 {
			t.Fatal("wrong facility type", m)
		}
		if prior := used[m.Day]; prior != "" && prior != m.ComponentKind {
			t.Fatal("distinct days were available but not preferred")
		}
		used[m.Day] = m.ComponentKind
	}
	if mins["lecture"] != 60 || mins["laboratory"] != 120 || r.PlacedMinutes != 180 {
		t.Fatal("unit multiplier changed", mins)
	}
	if err = Validate(s, r); err != nil {
		t.Fatal(err)
	}
	tampered := r
	tampered.Meetings = append([]domain.Meeting{}, r.Meetings...)
	tampered.Meetings[0].ComponentID = 999
	if Validate(s, tampered) == nil {
		t.Fatal("invalid component accepted")
	}
	tampered = r
	tampered.Meetings = r.Meetings[1:]
	if Validate(s, tampered) == nil {
		t.Fatal("incomplete component set accepted")
	}
}
func TestNoPartialMixedSubjectAndInvalidComponentTotals(t *testing.T) {
	s := splitFixture()
	s.Rooms = s.Rooms[:1]
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 0 || len(r.Meetings) != 0 || r.Issues[0].Code != "LACK_OF_FACILITY" {
		t.Fatalf("partial unit component kept %+v", r)
	}
	s = splitFixture()
	s.Components[0].WeeklyMinutes = 120
	r, _ = Generate(context.Background(), s, request())
	if r.Placed != 0 || r.Issues[0].Code != "INVALID_COMPONENTS" {
		t.Fatal(r)
	}
}
func TestRequestedExactLaboratoriesWaitForceAndManualOverride(t *testing.T) {
	s := splitFixture()
	s.Rooms = append(s.Rooms, domain.Room{ID: 3, BuildingID: 1, DepartmentID: 2, Code: "Foreign Computer Lab", Kind: "laboratory", Capacity: 50, Start: 420, End: 1140, Active: 1})
	s.FacilityRequests = []domain.FacilityRequest{{SubjectID: 1, RoomID: 3}}
	s.Sections = append(s.Sections, domain.Section{ID: 2, ProgramID: 2, Year: 1, Active: 1})
	r, _ := Generate(context.Background(), s, request())
	if r.Placed != 0 {
		t.Fatal("silently ignored exact foreign lab request")
	}
	req := request()
	req.BorrowPolicy = "wait"
	r, _ = Generate(context.Background(), s, req)
	if r.Placed != 0 {
		t.Fatal("Wait borrowed before owner completed")
	}
	s.Completions = []domain.DepartmentCompletion{{DepartmentID: 2, PeriodID: 1, Complete: true}}
	r, err := Generate(context.Background(), s, req)
	if err != nil || r.Placed != 1 || r.Borrowed == 0 {
		t.Fatal(r, err)
	}
	for _, m := range r.Meetings {
		if m.ComponentKind == "laboratory" && m.RoomID != 3 {
			t.Fatal("wrong lab", m)
		}
	}
	req = request()
	req.FacilityChoices = []FacilityChoice{{SectionID: 1, SubjectID: 1, RoomIDs: []int{2}}}
	r, _ = Generate(context.Background(), s, req)
	if r.Placed != 1 || r.Borrowed != 0 {
		t.Fatal("explicit compatible local override failed", r)
	}
	req.FacilityChoices[0].RoomIDs = []int{1}
	r, _ = Generate(context.Background(), s, req)
	if r.Placed != 0 {
		t.Fatal("classroom used in place of required lab")
	}
}
func TestWaitDoesNotDependOnCatalogOnlyDepartment(t *testing.T) {
	s := fixture()
	s.Rooms[0].DepartmentID = 2
	req := request()
	req.BorrowPolicy = "wait"
	r, err := Generate(context.Background(), s, req)
	if err != nil || r.Placed != 1 {
		t.Fatal("sectionless department blocked wait", r, err)
	}
}
func TestRetiredTimePreferencesDoNotChangeMeetings(t *testing.T) {
	s := fixture()
	before, _ := Generate(context.Background(), s, request())
	s.Preferences = []domain.FacultyPreference{{FacultyID: 1, FirstStart: 540, FirstEnd: 630, SecondStart: 780, SecondEnd: 870}}
	after, _ := Generate(context.Background(), s, request())
	if !reflect.DeepEqual(before.Meetings, after.Meetings) {
		t.Fatal("removed time preference affected schedule")
	}
}
func TestContinuationKeepsPlacementsAndProgressCounts(t *testing.T) {
	s := splitFixture()
	first, _ := Generate(context.Background(), s, request())
	req := request()
	req.Pinned = first.Meetings
	events := []Progress{}
	second, err := GenerateWithProgress(context.Background(), s, req, func(p Progress) { events = append(events, p) })
	if err != nil || !reflect.DeepEqual(first.Meetings, second.Meetings) {
		t.Fatal("valid pinned meetings changed", err)
	}
	last := -1
	for _, p := range events {
		if p.Processed < last || p.Processed > p.Total {
			t.Fatal("invalid progress", events)
		}
		last = p.Processed
	}
	if len(events) == 0 || events[len(events)-1].Stage != "finished" || last != second.Required {
		t.Fatal(events)
	}
}
func TestBorrowedFacilitiesPreferEdgesAndKeepHardConflicts(t *testing.T) {
	s := fixture()
	s.Rooms[0].DepartmentID = 2
	req := request()
	req.BorrowPolicy = "force"
	req.Mode = "force"
	r, _ := Generate(context.Background(), s, req)
	for _, m := range r.Meetings {
		if m.Start != 420 {
			t.Fatal("morning borrowed edge not preferred", m)
		}
	}
	s.Meetings = []domain.Meeting{{DepartmentID: 2, PeriodID: 1, SectionID: 5, SubjectID: 9, FacultyID: 2, RoomID: 1, Day: 1, Start: 420, End: 1140}}
	r, _ = Generate(context.Background(), s, req)
	if r.Placed != 0 {
		t.Fatal("Force ignored full reservation")
	}
}
