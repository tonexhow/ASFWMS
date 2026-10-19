package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

type FacilityChoice struct {
	SectionID int   `json:"section_id"`
	SubjectID int   `json:"subject_id"`
	RoomIDs   []int `json:"room_ids"`
}
type FacilityCandidate struct {
	ID           int    `json:"id"`
	Code         string `json:"code"`
	Kind         string `json:"kind"`
	BuildingID   int    `json:"building_id"`
	DepartmentID int    `json:"department_id"`
	OwnerReady   bool   `json:"owner_ready"`
	Borrowed     bool   `json:"borrowed"`
}
type Progress struct {
	Type      string `json:"type"`
	Stage     string `json:"stage"`
	Message   string `json:"message"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Placed    int    `json:"placed"`
	SectionID int    `json:"section_id,omitempty"`
	SubjectID int    `json:"subject_id,omitempty"`
	RoomID    int    `json:"room_id,omitempty"`
}

// Only active departments with an active reusable section bank must finish.
// GENED/shared catalogs without their own sections never hold the queue open.
func PendingDepartments(s domain.Snapshot, departmentID, periodID int) []int {
	owners := map[int]bool{}
	active := map[int]bool{}
	programs := map[int]int{}
	for _, d := range s.Departments {
		active[d.ID] = d.Active == 1
	}
	for _, p := range s.Programs {
		if p.Active == 1 {
			programs[p.ID] = p.DepartmentID
		}
	}
	for _, section := range s.Sections {
		d := programs[section.ProgramID]
		if section.Active == 1 && d > 0 && d != departmentID && active[d] {
			owners[d] = true
		}
	}
	for _, c := range s.Completions {
		if c.PeriodID == periodID && c.Complete {
			delete(owners, c.DepartmentID)
		}
	}
	out := []int{}
	for d := range owners {
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}
func (e *engine) ownerReady(dept int) bool {
	if dept == 0 || dept == e.r.DepartmentID {
		return true
	}
	for _, c := range e.s.Completions {
		if c.DepartmentID == dept && c.PeriodID == e.r.PeriodID && c.Complete {
			return true
		}
	}
	// Departments without an active section bank do not hold the Wait queue.
	owners := map[int]int{}
	for _, p := range e.s.Programs {
		if p.Active == 1 {
			owners[p.ID] = p.DepartmentID
		}
	}
	for _, section := range e.s.Sections {
		if section.Active == 1 && owners[section.ProgramID] == dept {
			return false
		}
	}
	return true
}
func (e *engine) mayBorrow(room domain.Room) bool {
	if room.DepartmentID == 0 || room.DepartmentID == e.r.DepartmentID {
		return true
	}
	policy := e.r.BorrowPolicy
	if policy == "" && e.r.Mode == "force" {
		policy = "force"
	}
	return policy == "force" || (policy == "wait" && len(PendingDepartments(e.s, e.r.DepartmentID, e.r.PeriodID)) == 0 && e.ownerReady(room.DepartmentID))
}
func componentRoom(kind string) string {
	switch kind {
	case "laboratory":
		return "laboratory"
	case "field":
		return "field"
	default:
		return "classroom"
	}
}
func (e *engine) componentRequirements(q Requirement) []Requirement {
	components := []domain.SubjectComponent{}
	for _, c := range e.s.Components {
		if c.SubjectID == q.Subject.ID {
			components = append(components, c)
		}
	}
	if len(components) == 0 {
		return []Requirement{q}
	} // legacy clients/fixtures, exact original requirements
	sort.SliceStable(components, func(i, j int) bool {
		rank := func(k string) int {
			if k == "lecture" {
				return 2
			}
			if k == "laboratory" {
				return 0
			}
			return 1
		}
		if rank(components[i].Kind) != rank(components[j].Kind) {
			return rank(components[i].Kind) < rank(components[j].Kind)
		}
		return components[i].SortOrder < components[j].SortOrder
	})
	out := []Requirement{}
	for _, c := range components {
		v := q
		v.ComponentID = c.ID
		v.ComponentKind = c.Kind
		v.Subject.Units = c.Units
		v.Subject.WeeklyMinutes = c.WeeklyMinutes
		v.Subject.RequiredRoom = componentRoom(c.Kind)
		if len(components) > 1 {
			v.Subject.Pattern = "auto"
			if named, ok := map[string]string{"mwf": "1,3,5", "tth": "2,4", "mw": "1,3", "mf": "1,5", "wf": "3,5", "sat": "6", "sun": "7"}[q.Subject.Pattern]; ok {
				allowed := []string{}
				for day := 1; day <= 7; day++ {
					if domain.ContainsDay(named, day) && e.dayAllowed(q.Subject, day) {
						allowed = append(allowed, fmt.Sprint(day))
					}
				}
				v.Subject.AllowedDays = strings.Join(allowed, ",")
			}
		}
		out = append(out, v)
	}
	return out
}

// An explicit per-preview choice overrides the subject's request only for the
// chosen facility type. It never changes capacity, availability or ownership.
func (e *engine) requestAllows(q Requirement, room domain.Room) bool {
	var ids []int
	for _, choice := range e.r.FacilityChoices {
		if choice.SectionID == q.Section.ID && choice.SubjectID == q.Subject.ID {
			for _, id := range choice.RoomIDs {
				if e.rooms[id].Kind == room.Kind {
					ids = append(ids, id)
				}
			}
		}
	}
	if len(ids) == 0 {
		for _, request := range e.s.FacilityRequests {
			if request.SubjectID == q.Subject.ID && e.rooms[request.RoomID].Kind == room.Kind {
				ids = append(ids, request.RoomID)
			}
		}
	}
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == room.ID {
			return true
		}
	}
	return false
}
func (e *engine) facilityIssue(q Requirement, message string) Issue {
	v := issue(q, "LACK_OF_FACILITY", message)
	seen := map[int]bool{}
	copyEngine := *e
	copyEngine.r.Mode = "force"
	copyEngine.r.BorrowPolicy = "force"
	for _, component := range e.componentRequirements(q) {
		for _, room := range copyEngine.candidateRooms(component) {
			if seen[room.ID] {
				continue
			}
			seen[room.ID] = true
			v.FacilityCandidates = append(v.FacilityCandidates, FacilityCandidate{ID: room.ID, Code: room.Code, Kind: room.Kind, BuildingID: room.BuildingID, DepartmentID: room.DepartmentID, OwnerReady: e.ownerReady(room.DepartmentID), Borrowed: room.DepartmentID != 0 && room.DepartmentID != e.r.DepartmentID})
		}
	}
	return v
}
func (e *engine) placeComponents(ctx context.Context, q Requirement, busy []domain.Meeting) ([]domain.Meeting, string, error) {
	components := e.componentRequirements(q)
	placed := []domain.Meeting{}
	used := map[int]bool{}
	for _, component := range components {
		occupied := append(append([]domain.Meeting{}, busy...), placed...)
		var meetings []domain.Meeting
		var reason string
		var err error
		// Different unit types prefer different teaching days (for example
		// laboratory Mon/Fri and lecture Wed). Fall back rather than invent a
		// hard restriction when the remaining days cannot fit the component.
		if len(components) > 1 && len(used) > 0 {
			separate := component
			ds := []string{}
			for day := 1; day <= 7; day++ {
				if !used[day] && e.dayAllowed(component.Subject, day) {
					ds = append(ds, fmt.Sprint(day))
				}
			}
			if len(ds) > 0 {
				separate.Subject.AllowedDays = strings.Join(ds, ",")
				separate.Subject.SaturdayFallback = 0
				meetings, reason, err = e.place(ctx, separate, occupied)
			}
		}
		if len(meetings) == 0 && err == nil {
			meetings, reason, err = e.place(ctx, component, occupied)
		}
		if err != nil || len(meetings) == 0 {
			return nil, reason, err
		}
		placed = append(placed, meetings...)
		for _, m := range meetings {
			used[m.Day] = true
		}
	}
	return placed, "", nil
}
func GenerateWithProgress(ctx context.Context, s domain.Snapshot, r Request, report func(Progress)) (Result, error) {
	started := time.Now()
	e, err := prepare(s, r)
	if err != nil {
		return Result{}, err
	}
	qs, issues := e.requirements()
	out, _ := Preflight(s, r)
	emit := func(stage, message string, processed int, q Requirement, room int) {
		if report != nil {
			report(Progress{Type: "progress", Stage: stage, Message: message, Processed: processed, Total: len(qs), Placed: out.Placed, SectionID: q.Section.ID, SubjectID: q.Subject.ID, RoomID: room})
		}
	}
	emit("starting", "Loaded curriculum requirements. Placing majors before minors; no reservations are committed.", 0, Requirement{}, 0)
	blocked := map[[2]int]bool{}
	for _, v := range issues {
		blocked[[2]int{v.SectionID, v.SubjectID}] = true
	}
	rand.New(rand.NewSource(r.Seed)).Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
	sort.SliceStable(qs, func(i, j int) bool {
		a, b := qs[i], qs[j]
		if (a.Subject.Priority == "major") != (b.Subject.Priority == "major") {
			return a.Subject.Priority == "major"
		}
		if a.Explicit != b.Explicit {
			return a.Explicit
		}
		return a.Faculty.PriorityScheduling > b.Faculty.PriorityScheduling
	})
	busy := append([]domain.Meeting{}, e.base...)
	pinned := map[[2]int][]domain.Meeting{}
	for _, m := range r.Pinned {
		k := [2]int{m.SectionID, m.SubjectID}
		pinned[k] = append(pinned[k], m)
	}
	// Preserve complete, still-valid section-subject groups before filling gaps.
	for _, q := range qs {
		k := [2]int{q.Section.ID, q.Subject.ID}
		ms := pinned[k]
		if len(ms) == 0 {
			continue
		}
		if err := e.validateGroup(q, ms, busy); err == nil {
			out.Meetings = append(out.Meetings, ms...)
			busy = append(busy, ms...)
		} else {
			delete(pinned, k)
		}
	}
	preserved := map[[2]int]bool{}
	for _, m := range out.Meetings {
		preserved[[2]int{m.SectionID, m.SubjectID}] = true
		out.PlacedMinutes += m.End - m.Start
		out.Borrowed += m.Borrowed
	}
	out.Placed = len(preserved)
	// An issue from preflight cannot survive a valid pinned placement.
	filtered := out.Issues[:0]
	for _, v := range out.Issues {
		if !preserved[[2]int{v.SectionID, v.SubjectID}] {
			filtered = append(filtered, v)
		}
	}
	out.Issues = filtered
	for i, q := range qs {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		key := [2]int{q.Section.ID, q.Subject.ID}
		emit("selecting", fmt.Sprintf("%s / %s: finding faculty, component facilities and free times.", q.Section.Code, q.Subject.Code), i, q, 0)
		if preserved[key] {
			emit("preserved", "Kept previously valid meetings unchanged.", i+1, q, 0)
			continue
		}
		if blocked[key] {
			emit("unresolved", "Input or facility restriction needs attention.", i+1, q, 0)
			continue
		}
		var placed []domain.Meeting
		var reason string
		if q.Explicit {
			placed, reason, err = e.placeComponents(ctx, q, busy)
		} else {
			failures := map[string]int{}
			for _, faculty := range e.eligibleFaculty(q) {
				q.Faculty = faculty
				placed, reason, err = e.placeComponents(ctx, q, busy)
				if err != nil || len(placed) > 0 {
					break
				}
				failures[reason]++
			}
			if len(placed) == 0 && err == nil {
				reason = topFailure(failures)
			}
		}
		if err != nil {
			return Result{}, err
		}
		if len(placed) == 0 && e.r.Mode != "force" && e.r.BorrowPolicy != "force" {
			probe := *e
			probe.r.Mode, probe.r.BorrowPolicy = "force", "force"
			probe.starts = map[string][]int{}
			candidates := []domain.Faculty{q.Faculty}
			if !q.Explicit {
				candidates = e.eligibleFaculty(q)
			}
			// A diagnostic attempt must not consume the main search budget.
			diagnostic, stop := context.WithTimeout(ctx, 350*time.Millisecond)
			for _, faculty := range candidates {
				test := q
				test.Faculty = faculty
				possible, _, probeErr := probe.placeComponents(diagnostic, test, busy)
				if probeErr != nil {
					break
				}
				borrowed := false
				for _, m := range possible {
					borrowed = borrowed || m.Borrowed == 1
				}
				if len(possible) > 0 && borrowed {
					reason = "LACK_OF_FACILITY"
					break
				}
			}
			stop()
		}
		if len(placed) == 0 {
			v := issue(q, reason, failureMessage(reason, q.Explicit))
			if reason == "LACK_OF_FACILITY" {
				v = e.facilityIssue(q, v.Message)
			}
			if reason == "LACK_INSTRUCTOR" || reason == "BREAK_CONFLICT" {
				for _, f := range e.eligibleFaculty(q) {
					if f.ID != q.Faculty.ID {
						v.Suggestions = append(v.Suggestions, f.ID)
					}
				}
			}
			out.Issues = append(out.Issues, v)
			emit("unresolved", reason+": "+v.Message, i+1, q, 0)
			continue
		}
		out.Placed++
		for _, m := range placed {
			out.PlacedMinutes += m.End - m.Start
			out.Borrowed += m.Borrowed
			emit("facility", fmt.Sprintf("Selected %s for %s, day %d, %02d:%02d-%02d:%02d.", m.Room, m.ComponentKind, m.Day, m.Start/60, m.Start%60, m.End/60, m.End%60), i, q, m.RoomID)
		}
		out.Meetings = append(out.Meetings, placed...)
		busy = append(busy, placed...)
		emit("placed", "All unit components placed for this section-subject.", i+1, q, 0)
	}
	out.Outcome = "partial"
	if len(out.Issues) == 0 && len(qs) > 0 && out.Placed == len(qs) {
		out.Outcome = "complete"
	}
	out.ElapsedMS = time.Since(started).Milliseconds()
	emit("finished", fmt.Sprintf("Preview ready: %d of %d section-subjects placed.", out.Placed, len(qs)), len(qs), Requirement{}, 0)
	return out, nil
}
func patternMatches(patterns [][]part, ms []domain.Meeting) bool {
	for _, parts := range patterns {
		if len(parts) != len(ms) {
			continue
		}
		want := map[[2]int]int{}
		for _, p := range parts {
			want[[2]int{p.day, p.duration}]++
		}
		for _, m := range ms {
			want[[2]int{m.Day, m.End - m.Start}]--
		}
		ok := true
		for _, n := range want {
			if n != 0 {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}
func (e *engine) validateGroup(q Requirement, ms, busy []domain.Meeting) error {
	if len(ms) == 0 || q.Subject.Active != 1 {
		return errors.New("teaching requirements changed; regenerate")
	}
	chosen := e.faculty[ms[0].FacultyID]
	if chosen.ID == 0 || chosen.Active != 1 || !e.s.Eligible(chosen.ID, q.Subject.ID) || (q.Explicit && chosen.ID != q.Faculty.ID) || (!q.Explicit && chosen.SectionPolicy == "exclusive") {
		return errors.New("preview instructor is no longer eligible or differs from the explicit assignment")
	}
	q.Faculty = chosen
	components := e.componentRequirements(q)
	expected := map[int]Requirement{}
	groups := map[int][]domain.Meeting{}
	total := 0
	for _, c := range components {
		expected[c.ComponentID] = c
	}
	for _, m := range ms {
		c, ok := expected[m.ComponentID]
		if !ok || m.ComponentKind != c.ComponentKind {
			return errors.New("subject unit components changed; regenerate")
		}
		if m.FacultyID != chosen.ID || m.DepartmentID != e.r.DepartmentID || m.PeriodID != e.r.PeriodID {
			return errors.New("invalid faculty, department or period")
		}
		legal := false
		for _, room := range e.candidateRooms(c) {
			if room.ID == m.RoomID {
				legal = true
				borrowed := 0
				if room.DepartmentID != 0 && room.DepartmentID != e.r.DepartmentID {
					borrowed = 1
				}
				if m.Borrowed != borrowed {
					return errors.New("invalid facility borrowing annotation")
				}
			}
		}
		if !legal {
			return errors.New("facility request, borrowing permission, capacity or type changed; regenerate")
		}
		groups[m.ComponentID] = append(groups[m.ComponentID], m)
		total += m.End - m.Start
	}
	if total != q.Subject.WeeklyMinutes || len(groups) != len(components) {
		return errors.New("all component contact minutes must be placed together")
	}
	for id, c := range expected {
		minutes := 0
		for _, m := range groups[id] {
			minutes += m.End - m.Start
		}
		if minutes != c.Subject.WeeklyMinutes || !patternMatches(e.patterns(c), groups[id]) {
			return fmt.Errorf("%s %s component minutes or meeting pattern changed; regenerate", q.Subject.Code, c.ComponentKind)
		}
	}
	if why := e.check(q, ms, busy); why != "" {
		return fmt.Errorf("preview no longer feasible: %s", why)
	}
	return nil
}
func Validate(s domain.Snapshot, result Result) error {
	e, err := prepare(s, result.Request)
	if err != nil {
		return err
	}
	qs, _ := e.requirements()
	lookup := map[[2]int]Requirement{}
	for _, q := range qs {
		lookup[[2]int{q.Section.ID, q.Subject.ID}] = q
	}
	groups := map[[2]int][]domain.Meeting{}
	order := [][2]int{}
	for _, m := range result.Meetings {
		k := [2]int{m.SectionID, m.SubjectID}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], m)
	}
	busy := append([]domain.Meeting{}, e.base...)
	for _, k := range order {
		q, ok := lookup[k]
		if !ok {
			return errors.New("teaching requirements changed; regenerate")
		}
		if err := e.validateGroup(q, groups[k], busy); err != nil {
			return err
		}
		busy = append(busy, groups[k]...)
	}
	if len(result.Meetings) == 0 {
		return errors.New("an empty preview cannot replace committed schedules")
	}
	if result.Outcome == "complete" && (len(groups) != len(qs) || len(result.Issues) > 0) {
		return errors.New("incomplete preview cannot be marked complete")
	}
	return nil
}
