// Package scheduling is a bounded, deterministic priority scheduler with no storage dependencies.
package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/facilities"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Request struct {
	DepartmentID    int              `json:"department_id"`
	PeriodID        int              `json:"period_id"`
	Mode            string           `json:"mode"`
	Seed            int64            `json:"seed"`
	BorrowPolicy    string           `json:"borrow_policy,omitempty"`
	ConfirmBorrow   string           `json:"confirm_borrow,omitempty"`
	SourceRunID     int              `json:"source_run_id,omitempty"`
	FacilityChoices []FacilityChoice `json:"facility_choices,omitempty"`
	Pinned          []domain.Meeting `json:"-"`
}
type Issue struct {
	Code               string              `json:"code"`
	Message            string              `json:"message"`
	SectionID          int                 `json:"section_id,omitempty"`
	SubjectID          int                 `json:"subject_id,omitempty"`
	FacultyID          int                 `json:"faculty_id,omitempty"`
	Section            string              `json:"section,omitempty"`
	Subject            string              `json:"subject,omitempty"`
	SubjectTitle       string              `json:"subject_title,omitempty"`
	Minutes            int                 `json:"minutes"`
	Suggestions        []int               `json:"suggestions,omitempty"`
	FacilityCandidates []FacilityCandidate `json:"facility_candidates,omitempty"`
}
type Requirement struct {
	Section domain.Section
	Subject domain.Subject
	Faculty domain.Faculty
	// Explicit is set only for faculty->subject->section selections.
	Explicit       bool
	ComponentID    int
	ComponentKind  string
	BorrowedSearch bool
}
type Result struct {
	Request         Request          `json:"request"`
	Outcome         string           `json:"outcome"`
	Required        int              `json:"required"`
	Placed          int              `json:"placed"`
	RequiredMinutes int              `json:"required_minutes"`
	PlacedMinutes   int              `json:"placed_minutes"`
	Borrowed        int              `json:"borrowed"`
	Replacing       int              `json:"replacing"`
	ElapsedMS       int64            `json:"elapsed_ms"`
	Meetings        []domain.Meeting `json:"meetings"`
	Issues          []Issue          `json:"issues"`
	Warnings        []Issue          `json:"warnings"`
}
type engine struct {
	s                domain.Snapshot
	r                Request
	p                domain.Period
	subjects         map[int]domain.Subject
	faculty          map[int]domain.Faculty
	rooms            map[int]domain.Room
	buildings        map[int]domain.Building
	departments      map[int]domain.Department
	periods          map[int]domain.Period
	base             []domain.Meeting
	workByFaculty    map[int]map[int][]domain.Window
	blockedByFaculty map[int]map[int][]domain.Window
	preferences      map[int]domain.FacultyPreference
	starts           map[string][]int
}

func prepare(s domain.Snapshot, r Request) (*engine, error) {
	if r.BorrowPolicy != "" && r.BorrowPolicy != "none" && r.BorrowPolicy != "wait" && r.BorrowPolicy != "force" {
		return nil, errors.New("invalid borrowing policy")
	}
	if r.Mode != "standard" && r.Mode != "force" {
		return nil, errors.New("choose standard or force generation")
	}
	e := &engine{s: s, r: r, subjects: map[int]domain.Subject{}, faculty: map[int]domain.Faculty{}, rooms: map[int]domain.Room{}, buildings: map[int]domain.Building{}, departments: map[int]domain.Department{}, periods: map[int]domain.Period{}, workByFaculty: map[int]map[int][]domain.Window{}, blockedByFaculty: map[int]map[int][]domain.Window{}, preferences: map[int]domain.FacultyPreference{}, starts: map[string][]int{}}
	for _, w := range s.WorkTimes {
		if e.workByFaculty[w.FacultyID] == nil {
			e.workByFaculty[w.FacultyID] = map[int][]domain.Window{}
		}
		e.workByFaculty[w.FacultyID][w.Day] = append(e.workByFaculty[w.FacultyID][w.Day], w)
	}
	for _, w := range s.Blocks {
		if e.blockedByFaculty[w.FacultyID] == nil {
			e.blockedByFaculty[w.FacultyID] = map[int][]domain.Window{}
		}
		e.blockedByFaculty[w.FacultyID][w.Day] = append(e.blockedByFaculty[w.FacultyID][w.Day], w)
	}
	for _, preference := range s.Preferences {
		e.preferences[preference.FacultyID] = preference
	}
	for _, v := range s.Departments {
		e.departments[v.ID] = v
	}
	if d, ok := e.departments[r.DepartmentID]; !ok || d.Active != 1 {
		return nil, errors.New("select an active department")
	}
	for _, v := range s.Periods {
		e.periods[v.ID] = v
	}
	p, ok := e.periods[r.PeriodID]
	if !ok || p.Active != 1 {
		return nil, errors.New("select an active academic period")
	}
	e.p = p
	for _, v := range s.Subjects {
		e.subjects[v.ID] = v
	}
	for _, v := range s.Faculty {
		e.faculty[v.ID] = v
	}
	for _, v := range s.Rooms {
		e.rooms[v.ID] = v
	}
	for _, v := range s.Buildings {
		e.buildings[v.ID] = v
	}
	for _, m := range s.Meetings {
		if m.DepartmentID == r.DepartmentID && m.PeriodID == r.PeriodID {
			continue
		}
		other := e.periods[m.PeriodID]
		if DatesOverlap(p, other) {
			e.base = append(e.base, m)
		}
	}
	return e, nil
}
func DatesOverlap(a, b domain.Period) bool {
	if a.AcademicYear != "" && b.AcademicYear != "" {
		return a.AcademicYear == b.AcademicYear && domain.SemestersMatch(a.Semester, b.Semester)
	}
	return a.StartDate <= b.EndDate && b.StartDate <= a.EndDate
}
func issue(q Requirement, code, message string) Issue {
	return Issue{Code: code, Message: message, SectionID: q.Section.ID, SubjectID: q.Subject.ID, FacultyID: q.Faculty.ID, Section: q.Section.Code, Subject: q.Subject.Code, SubjectTitle: q.Subject.Title, Minutes: q.Subject.WeeklyMinutes}
}
func (e *engine) requirements() (all []Requirement, issues []Issue) {
	programs := map[int]domain.Program{}
	curricula := map[int]domain.Curriculum{}
	assignments := map[[2]int]int{}
	for _, v := range e.s.Programs {
		programs[v.ID] = v
	}
	for _, v := range e.s.Curricula {
		curricula[v.ID] = v
	}
	for _, v := range e.s.Assignments {
		assignments[[2]int{v.SectionID, v.SubjectID}] = v.FacultyID
	}
	sections := 0
	for _, sc := range e.s.Sections {
		pr := programs[sc.ProgramID]
		if sc.Active != 1 || pr.DepartmentID != e.r.DepartmentID {
			continue
		}
		sections++
		if pr.Active != 1 {
			issues = append(issues, Issue{Code: "ACADEMIC_LINK", SectionID: sc.ID, Section: sc.Code,
				Message: fmt.Sprintf("%s belongs to an inactive program (%s). Activate the program or deactivate this section before generation.", sc.Code, pr.Code)})
			continue
		}
		// The section bank is reused every academic year and semester. Resolve
		// the curriculum for *this request*, never from the stored legacy links.
		curriculumID, matchErr := domain.ResolveSectionCurriculum(e.s, sc.ProgramID, sc.Year, e.r.PeriodID)
		if matchErr != nil {
			code := "MISSING_CURRICULUM"
			if strings.Contains(matchErr.Error(), "multiple") {
				code = "AMBIGUOUS_CURRICULUM"
			}
			issues = append(issues, Issue{Code: code, SectionID: sc.ID, Section: sc.Code,
				Message: fmt.Sprintf("%s / %s / %s: %s", pr.Code, domain.YearLabel(sc.Year), e.p.Semester, matchErr.Error())})
			continue
		}
		cu := curricula[curriculumID]
		if cu.Active != 1 {
			continue
		}
		count := 0
		seen := map[int]bool{}
		for _, mp := range e.s.Mappings {
			if mp.CurriculumID != curriculumID || mp.Year != sc.Year || seen[mp.SubjectID] {
				continue
			}
			seen[mp.SubjectID] = true
			count++
			su := e.subjects[mp.SubjectID]
			assignedID, explicit := assignments[[2]int{sc.ID, su.ID}]
			f := e.faculty[assignedID]
			sc.CurriculumID = curriculumID // annotate this requirement's resolved curriculum only
			q := Requirement{Section: sc, Subject: su, Faculty: f, Explicit: explicit}
			if !explicit {
				// Unselected sections remain eligible for automatic allocation.
				// This does not create or modify an explicit teaching assignment.
				candidates := e.eligibleFaculty(q)
				if len(candidates) != 0 {
					q.Faculty = candidates[0]
				}
			}

			all = append(all, q)
			if su.ID == 0 || su.Active != 1 {
				issues = append(issues, issue(q, "INACTIVE_SUBJECT", "A required curriculum subject is missing or inactive."))
				continue
			}
			if q.Faculty.ID == 0 || q.Faculty.Active != 1 {
				message := "No available faculty is eligible for automatic placement. Faculty set to Handle only selected sections are excluded from other sections. Link subject eligibility or add an explicit assignment."
				if explicit {
					message = "The explicitly selected instructor is missing or inactive. Change this assignment before generation."
				}
				v := issue(q, "MISSING_INSTRUCTOR", message)
				for _, x := range e.s.Eligibility {
					if x.SubjectID == su.ID && e.faculty[x.FacultyID].Active == 1 {
						v.Suggestions = append(v.Suggestions, x.FacultyID)
					}
				}
				issues = append(issues, v)
				continue
			}
			if !e.s.Eligible(q.Faculty.ID, su.ID) {
				issues = append(issues, issue(q, "INELIGIBLE_INSTRUCTOR", "The assigned instructor must have this subject in their eligible subject list."))
				continue
			}
			if su.WeeklyMinutes <= 0 || su.WeeklyMinutes%15 != 0 {
				issues = append(issues, issue(q, "INVALID_DURATION", "Contact minutes must be positive multiples of 15; no rounding is performed."))
				continue
			}
			componentMinutes, hasComponents := 0, false
			validComponents := true
			for _, c := range e.s.Components {
				if c.SubjectID != su.ID {
					continue
				}
				hasComponents = true
				componentMinutes += c.WeeklyMinutes
				if c.WeeklyMinutes <= 0 || c.WeeklyMinutes%15 != 0 || float64(c.WeeklyMinutes) != c.Units*60 || (c.Kind != "lecture" && c.Kind != "laboratory" && c.Kind != "field") {
					validComponents = false
				}
			}
			if hasComponents && (!validComponents || componentMinutes != su.WeeklyMinutes) {
				issues = append(issues, issue(q, "INVALID_COMPONENTS", "Unit components must total the subject's contact minutes: one unit is 60 minutes per week."))
				continue
			}
			for _, part := range e.componentRequirements(q) {
				if len(e.patterns(part)) == 0 {
					issues = append(issues, issue(q, "PATTERN_UNSUPPORTED", "A unit component cannot fit the permitted days and contact time. Review its units and meeting days."))
					break
				}
				if len(e.candidateRooms(part)) == 0 {
					issues = append(issues, e.facilityIssue(q, "No permitted facility matches a required component, capacity and requested facilities. Review candidates; Wait or Force never overrides conflicts."))
					break
				}
			}
		}
		if count == 0 {
			issues = append(issues, Issue{Code: "EMPTY_CURRICULUM", Message: fmt.Sprintf("No subjects are linked to %s / %s / %s. Open Academic > Curricula and link this semester's subjects.", pr.Code, domain.YearLabel(sc.Year), e.p.Semester), SectionID: sc.ID, Section: sc.Code})
		}
	}
	if sections == 0 {
		issues = append(issues, Issue{Code: "NO_SECTIONS", Message: "This department has no active sections in its reusable Section bank. Add sections under Academic > Sections."})
	}
	return all, issues
}

// adviserWarnings are reminders, never placement constraints. Missing
// advisers do not affect schedule completeness or instructor allocation.
func (e *engine) adviserWarnings() []Issue {
	warnings := []Issue{}
	programs := map[int]domain.Program{}
	for _, p := range e.s.Programs {
		programs[p.ID] = p
	}
	for _, section := range e.s.Sections {
		if section.Active != 1 || programs[section.ProgramID].DepartmentID != e.r.DepartmentID {
			continue
		}
		if section.AdviserID == 0 {
			warnings = append(warnings, Issue{Code: "MISSING_ADVISER", SectionID: section.ID, Section: section.Code,
				Message: "No adviser has been assigned to this section. You can generate and commit its timetable now, but assign an adviser when one is available."})
		}
	}
	return warnings
}

func Preflight(s domain.Snapshot, r Request) (Result, error) {
	e, err := prepare(s, r)
	if err != nil {
		return Result{}, err
	}
	qs, issues := e.requirements()
	out := Result{Request: r, Required: len(qs), Meetings: []domain.Meeting{}, Issues: issues, Warnings: e.adviserWarnings(), Outcome: "partial"}
	if out.Issues == nil {
		out.Issues = []Issue{}
	}
	for _, q := range qs {
		out.RequiredMinutes += q.Subject.WeeklyMinutes
	}
	for _, m := range s.Meetings {
		if m.DepartmentID == r.DepartmentID && m.PeriodID == r.PeriodID {
			out.Replacing++
		}
	}
	return out, nil
}
func Generate(ctx context.Context, s domain.Snapshot, r Request) (Result, error) {
	return GenerateWithProgress(ctx, s, r, nil)
}

// Issue codes state the limiting resource rather than exposing search internals.
func failureMessage(reason string, explicit bool) string {
	assigned := " The explicit instructor remains assigned; select another instructor in Faculty / Sections to change that choice."
	if !explicit {
		assigned = " The system tried every eligible instructor without fixing a new section assignment."
	}
	switch reason {
	case "LACK_OF_FACILITY", "FACILITY_CONFLICT", "NO_COMPATIBLE_ROOM":
		return "No compatible, available room could host the required meetings. Review facilities, capacity, opening hours, and other departments' reservations. Force mode can borrow an unoccupied room."
	case "LACK_INSTRUCTOR", "INSTRUCTOR_CONFLICT", "INSTRUCTOR_AVAILABILITY", "FACULTY_WORKLOAD", "FACULTY_UNAVAILABLE":
		return "No eligible instructor has a free, permitted working-time combination for all required weekly meetings. Review subject eligibility, instructor availability and occupied classes." + assigned
	case "SECTION_CONFLICT", "SECTION_CONSECUTIVE_LIMIT":
		return "The section has conflicting classes or would exceed three consecutive subjects without a break. Review the section's other meeting times."
	case "BREAK_CONFLICT", "LUNCH_BREAK":
		return "The required meetings cannot fit while preserving the instructor's lunch break (at least 60 minutes within the lunch window). Review free hours or select another capable instructor." + assigned
	case "TIME_WINDOW", "DAY_RESTRICTION":
		return "The required meeting pattern cannot fit the subject's permitted days, facility hours and academic period. Review meeting days and hours."
	default:
		return "No conflict-free combination fits the current instructor, section, facility and meeting-day constraints. Review the listed requirements and available resources." + assigned
	}
}
func topFailure(counts map[string]int) string {
	if len(counts) == 0 {
		return "NO_FEASIBLE_SLOT"
	}
	var best string
	bestCount := 0
	for code, n := range counts {
		if n > bestCount || (n == bestCount && code < best) {
			best = code
			bestCount = n
		}
	}
	return best
}

// eligibleFaculty lists eligible instructors in a stable, auditable order.
// Department matches and consideration priority are soft preferences; placement
// still enforces all hard availability and workload restrictions.
func (e *engine) eligibleFaculty(q Requirement) []domain.Faculty {
	ids := map[int]bool{}
	list := []domain.Faculty{}
	for _, x := range e.s.Eligibility {
		f := e.faculty[x.FacultyID]
		if x.SubjectID != q.Subject.ID || f.ID == 0 || f.Active != 1 || ids[f.ID] || f.SectionPolicy == "exclusive" {
			continue
		}
		ids[f.ID] = true
		list = append(list, f)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.PriorityScheduling != b.PriorityScheduling {
			return a.PriorityScheduling > b.PriorityScheduling
		}
		aHome, bHome := a.DepartmentID == e.r.DepartmentID, b.DepartmentID == e.r.DepartmentID
		if aHome != bHome {
			return aHome
		}
		return a.ID < b.ID
	})
	return list
}

func (e *engine) candidateRooms(q Requirement) []domain.Room {
	out := []domain.Room{}
	for _, r := range e.s.Rooms {
		if r.Active != 1 || e.buildings[r.BuildingID].Active != 1 {
			continue
		}
		if r.DepartmentID != 0 && e.departments[r.DepartmentID].Active != 1 {
			continue
		}
		if !e.mayBorrow(r) {
			continue
		}
		if q.Section.Size > 0 && r.Capacity > 0 && r.Capacity < q.Section.Size {
			continue
		}
		if q.Subject.RequiredRoom != "any" && r.Kind != q.Subject.RequiredRoom {
			continue
		}
		if !e.requestAllows(q, r) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := roomRank(out[i], e.r.DepartmentID), roomRank(out[j], e.r.DepartmentID)
		if a != b {
			return a < b
		}
		preferredID := e.preferences[q.Faculty.ID].PreferredRoomID
		aPreferred, bPreferred := out[i].ID == preferredID && preferredID > 0, out[j].ID == preferredID && preferredID > 0
		if aPreferred != bPreferred {
			return aPreferred
		}
		return out[i].ID < out[j].ID
	})
	// A legacy import may contain two active IDs for one physical room
	// (for example CITE-1 and ITLAB1). The preference/ranking above decides
	// which label wins, but the scheduler must not present them as extra rooms.
	unique := make([]domain.Room, 0, len(out))
	seenPhysical := map[string]bool{}
	for _, r := range out {
		key := facilities.Key(r.BuildingID, r.ID, r.Kind, r.Name)
		if seenPhysical[key] {
			continue
		}
		seenPhysical[key] = true
		unique = append(unique, r)
	}
	return unique
}
func roomRank(r domain.Room, dept int) int {
	if r.DepartmentID == dept {
		return 0
	}
	if r.DepartmentID == 0 {
		return 1
	}
	return 2
}

type part struct{ day, duration int }

func (e *engine) dayAllowed(s domain.Subject, day int) bool {
	if day < 1 || day > 7 {
		return false
	}
	return domain.ContainsDay(s.AllowedDays, day) || (day == 6 && s.SaturdayFallback == 1)
}
func (e *engine) patterns(q Requirement) [][]part {
	s := q.Subject
	total := s.WeeklyMinutes
	primary, fallback := [][]part{}, [][]part{}
	seen := map[string]bool{}
	// Saturday fallback is searched only AFTER every primary room/day candidate.
	add := func(ds []int, distribute bool) {
		if len(ds) == 0 || total <= 0 || total%15 != 0 {
			return
		}
		if !distribute && total%len(ds) != 0 {
			return
		}
		base := (total / 15) / len(ds)
		extra := (total / 15) % len(ds)
		if !distribute && extra != 0 {
			return
		}
		ps := []part{}
		isFallback := false
		for i, day := range ds {
			if !e.dayAllowed(s, day) {
				return
			}
			duration := base * 15
			if i < extra {
				duration += 15
			}
			if duration < 15 || duration > 180 {
				return
			}
			ps = append(ps, part{day, duration})
			if day == 6 && !domain.ContainsDay(s.AllowedDays, 6) {
				isFallback = true
			}
		}
		key := fmt.Sprint(ps)
		if seen[key] {
			return
		}
		seen[key] = true
		if isFallback {
			fallback = append(fallback, ps)
		} else {
			primary = append(primary, ps)
		}
	}
	named := map[string][]int{"mwf": {1, 3, 5}, "tth": {2, 4}, "mw": {1, 3}, "mf": {1, 5}, "wf": {3, 5}, "sat": {6}, "sun": {7}}
	selected, namedPattern := named[s.Pattern]
	if namedPattern {
		add(selected, false)
	} else {
		if s.Pattern == "auto" {
			if total == 120 {
				for _, ds := range [][]int{{1, 5}, {3, 5}, {1, 3}, {2, 4}} {
					add(ds, false)
				}
			} else {
				add([]int{1, 3, 5}, false)
				add([]int{2, 4}, false)
			}
		}
		for day := 1; day <= 7; day++ {
			add([]int{day}, false)
		}
		if s.Pattern != "single" {
			allowed := []int{}
			for day := 1; day <= 7; day++ {
				if e.dayAllowed(s, day) {
					allowed = append(allowed, day)
				}
			}
			// A normal weekly class needs at most three meetings. Longer
			// courses require as many days as the 180-minute/day cap needs.
			maxDays := 3
			if needed := (total + 179) / 180; needed > maxDays {
				maxDays = needed
			}
			if maxDays > len(allowed) {
				maxDays = len(allowed)
			}
			for count := 2; count <= maxDays; count++ {
				for mask := 1; mask < (1 << len(allowed)); mask++ {
					ds := []int{}
					for i, day := range allowed {
						if mask&(1<<i) != 0 {
							ds = append(ds, day)
						}
					}
					if len(ds) == count {
						add(ds, true)
					}
				}
			}
		}
	}
	if s.SaturdayFallback == 1 && !domain.ContainsDay(s.AllowedDays, 6) {
		add([]int{6}, false)
	}
	return append(primary, fallback...)
}
func fallbackPattern(s domain.Subject, parts []part) bool {
	if s.SaturdayFallback != 1 || domain.ContainsDay(s.AllowedDays, 6) {
		return false
	}
	for _, p := range parts {
		if p.day == 6 {
			return true
		}
	}
	return false
}
func (e *engine) meeting(q Requirement, r domain.Room, p part, start int) domain.Meeting {
	b := 0
	if r.DepartmentID != 0 && r.DepartmentID != e.r.DepartmentID {
		b = 1
	}
	owner := "Shared"
	if r.DepartmentID != 0 {
		owner = e.departments[r.DepartmentID].Code
	}
	return domain.Meeting{ComponentID: q.ComponentID, ComponentKind: q.ComponentKind, DepartmentID: e.r.DepartmentID, PeriodID: e.r.PeriodID, SectionID: q.Section.ID, SubjectID: q.Subject.ID, FacultyID: q.Faculty.ID, RoomID: r.ID, Day: p.day, Start: start, End: start + p.duration, Borrowed: b, Section: q.Section.Code, Subject: q.Subject.Code, Title: q.Subject.Title, Faculty: q.Faculty.Name, Room: r.Code, Owner: owner}
}

// quickWindowPossible prunes impossible day/room/faculty combinations before
// the costly collision/gap checks. It never declares a placement feasible.
func (e *engine) quickWindowPossible(q Requirement, room domain.Room, parts []part) bool {
	custom := len(e.workByFaculty[q.Faculty.ID]) > 0
	for _, p := range parts {
		from, to := e.p.DayStart, e.p.DayEnd
		if room.Start > from {
			from = room.Start
		}
		if room.End < to {
			to = room.End
		}
		if to-from < p.duration {
			return false
		}
		if !custom {
			if p.day < 1 || p.day > 5 {
				return false
			}
			if from < 420 {
				from = 420
			}
			if to > 1140 {
				to = 1140
			}
			if to-from < p.duration {
				return false
			}
		} else {
			available := false
			for _, w := range e.workByFaculty[q.Faculty.ID][p.day] {
				a, b := from, to
				if w.Start > a {
					a = w.Start
				}
				if w.End < b {
					b = w.End
				}
				if b-a >= p.duration {
					available = true
					break
				}
			}
			if !available {
				return false
			}
		}
	}
	return true
}

// candidateTimes ranks school-day preferences without turning them into
// prohibitions. Major subjects favor 08-12 / 13-18; minor subjects may use
// the edge windows (07-08, 12-13, 18-19) before taking prime time.
func (e *engine) candidateTimes(q Requirement, duration int, _ []domain.Meeting) []int {
	key := fmt.Sprintf("%d/%d/%d/%t/%t", duration, e.p.DayStart, e.p.DayEnd, q.Subject.Priority == "major", q.BorrowedSearch)
	if v, ok := e.starts[key]; ok {
		return v
	}
	type item struct{ time, score int }
	entries := []item{}
	for t := e.p.DayStart; t+duration <= e.p.DayEnd; t += 15 {
		end := t + duration
		edge := 0
		windows := [][2]int{{420, 480}, {720, 780}, {1080, 1140}}
		if q.BorrowedSearch {
			windows = [][2]int{{420, 510}, {690, 780}, {1050, 1140}}
		}
		for _, win := range windows {
			a, b := t, end
			if a < win[0] {
				a = win[0]
			}
			if b > win[1] {
				b = win[1]
			}
			if b > a {
				edge += b - a
			}
		}
		score := 0
		if q.Subject.Priority == "major" && !q.BorrowedSearch {
			score = edge * 9
		} else {
			score = (duration - edge) * 4
		}

		if t%60 != 0 {
			score += 8
		}
		entries = append(entries, item{t, score})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score {
			return entries[i].score < entries[j].score
		}
		return entries[i].time < entries[j].time
	})
	out := make([]int, 0, len(entries))
	for _, v := range entries {
		out = append(out, v.time)
	}
	e.starts[key] = out
	return out
}

// place caches legal day/time combinations for each room. A weekly pattern
// uses distinct days, so it can be constructed from independently feasible
// daily placements, then verified as one complete requirement. This searches
// the full finite space without a misleading, arbitrary 1,600-check cutoff.
func (e *engine) place(ctx context.Context, q Requirement, busy []domain.Meeting) ([]domain.Meeting, string, error) {
	rooms := e.candidateRooms(q)
	if len(rooms) == 0 {
		return nil, "LACK_OF_FACILITY", nil
	}
	patterns := e.patterns(q)
	if len(patterns) == 0 {
		return nil, "TIME_WINDOW", nil
	}
	reasons := map[string]int{}
	type slotKey struct{ room, day, duration int }
	cache := map[slotKey][]int{}
	available := func(room domain.Room, p part) ([]int, error) {
		key := slotKey{room.ID, p.day, p.duration}
		if val, exists := cache[key]; exists {
			return val, nil
		}
		times := []int{}
		search := q
		search.BorrowedSearch = room.DepartmentID != 0 && room.DepartmentID != e.r.DepartmentID
		for _, start := range e.candidateTimes(search, p.duration, busy) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			candidate := e.meeting(q, room, p, start)
			if why := e.check(q, []domain.Meeting{candidate}, busy); why == "" {
				times = append(times, start)
			} else {
				reasons[why]++
			}
		}
		cache[key] = times
		return times, nil
	}
	for phase := 0; phase < 2; phase++ {
		for _, room := range rooms {
			for _, parts := range patterns {
				if fallbackPattern(q.Subject, parts) != (phase == 1) {
					continue
				}
				if !e.quickWindowPossible(q, room, parts) {
					reasons["INSTRUCTOR_AVAILABILITY"]++
					continue
				}
				availableTimes := make([][]int, 0, len(parts))
				feasible := true
				for _, p := range parts {
					found, err := available(room, p)
					if err != nil {
						return nil, "", err
					}
					if len(found) == 0 {
						feasible = false
						break
					}
					availableTimes = append(availableTimes, found)
				}
				if !feasible {
					continue
				}
				// Try a common start across all meeting days first.
				choice := make([]int, len(parts))
				matched := false
				for _, start := range availableTimes[0] {
					valid := true
					for idx := 1; idx < len(parts); idx++ {
						ok := false
						for _, v := range availableTimes[idx] {
							if v == start {
								ok = true
								break
							}
						}
						if !ok {
							valid = false
							break
						}
					}
					if valid {
						for idx := range choice {
							choice[idx] = start
						}
						matched = true
						break
					}
				}
				if !matched {
					for idx := range choice {
						choice[idx] = availableTimes[idx][0]
					}
				}
				proposed := make([]domain.Meeting, 0, len(parts))
				for i, p := range parts {
					proposed = append(proposed, e.meeting(q, room, p, choice[i]))
				}
				if why := e.check(q, proposed, busy); why == "" {
					return proposed, "", nil
				} else {
					reasons[why]++
				}
			}
		}
	}
	grouped := map[string]int{}
	for key, n := range reasons {
		code := "NO_FEASIBLE_SLOT"
		switch key {
		case "FACULTY_AVAILABILITY", "FACULTY_UNAVAILABLE", "FACULTY_WORKLOAD", "INSTRUCTOR_AVAILABILITY", "INSTRUCTOR_CONFLICT":
			code = "LACK_INSTRUCTOR"
		case "FACILITY_CONFLICT", "NO_COMPATIBLE_ROOM":
			code = "LACK_OF_FACILITY"
		case "SECTION_CONFLICT", "SECTION_CONSECUTIVE_LIMIT":
			code = "SECTION_CONFLICT"
		case "LUNCH_BREAK":
			code = "BREAK_CONFLICT"
		case "TIME_WINDOW", "DAY_RESTRICTION":
			code = "TIME_WINDOW"
		}
		grouped[code] += n
	}
	return nil, topFailure(grouped), nil
}

func overlaps(a, b domain.Meeting) bool { return a.Day == b.Day && a.Start < b.End && b.Start < a.End }
func (e *engine) check(q Requirement, proposed, busy []domain.Meeting) string {
	all := append(append([]domain.Meeting{}, busy...), proposed...)
	for i, m := range proposed {
		r, ok := e.rooms[m.RoomID]
		if !ok || r.Active != 1 || e.buildings[r.BuildingID].Active != 1 {
			return "NO_COMPATIBLE_ROOM"
		}
		if m.End <= m.Start || m.End-m.Start > 180 || m.Start < e.p.DayStart || m.End > e.p.DayEnd || m.Start < r.Start || m.End > r.End {
			return "TIME_WINDOW"
		}
		if !e.dayAllowed(q.Subject, m.Day) {
			return "DAY_RESTRICTION"
		}
		working := false
		if len(e.workByFaculty[m.FacultyID]) == 0 {
			working = m.Day >= 1 && m.Day <= 5 && m.Start >= 420 && m.End <= 1140
		} else {
			for _, w := range e.workByFaculty[m.FacultyID][m.Day] {
				if w.Start <= m.Start && w.End >= m.End {
					working = true
					break
				}
			}
		}
		if !working {
			return "FACULTY_AVAILABILITY"
		}
		for _, w := range e.blockedByFaculty[m.FacultyID][m.Day] {
			if w.Start < m.End && m.Start < w.End {
				return "FACULTY_UNAVAILABLE"
			}
		}
		for _, x := range busy {
			if !overlaps(m, x) {
				continue
			}
			if m.FacultyID == x.FacultyID {
				return "INSTRUCTOR_CONFLICT"
			}
			if m.SectionID == x.SectionID {
				return "SECTION_CONFLICT"
			}
			if m.RoomID == x.RoomID {
				return "FACILITY_CONFLICT"
			}
			// Treat active aliases of one physical facility as one occupied space,
			// including when earlier committed meetings use the old room code.
			if other, ok := e.rooms[x.RoomID]; ok && facilities.Key(r.BuildingID, r.ID, r.Kind, r.Name) == facilities.Key(other.BuildingID, other.ID, other.Kind, other.Name) {
				return "FACILITY_CONFLICT"
			}
		}
		for j := 0; j < i; j++ {
			x := proposed[j]
			if !overlaps(m, x) {
				continue
			}
			if m.FacultyID == x.FacultyID {
				return "INSTRUCTOR_CONFLICT"
			}
			if m.SectionID == x.SectionID {
				return "SECTION_CONFLICT"
			}
			if m.RoomID == x.RoomID {
				return "FACILITY_CONFLICT"
			}
			if other, ok := e.rooms[x.RoomID]; ok && facilities.Key(r.BuildingID, r.ID, r.Kind, r.Name) == facilities.Key(other.BuildingID, other.ID, other.Kind, other.Name) {
				return "FACILITY_CONFLICT"
			}
		}
	}
	if q.Faculty.MaxWeeklyMinutes > 0 {
		points := []string{e.p.StartDate}
		for _, p := range e.periods {
			if p.StartDate >= e.p.StartDate && p.StartDate <= e.p.EndDate {
				points = append(points, p.StartDate)
			}
		}
		for _, d := range points {
			minutes := 0
			for _, m := range all {
				p := e.periods[m.PeriodID]
				if m.FacultyID == q.Faculty.ID && p.StartDate <= d && d <= p.EndDate {
					minutes += m.End - m.Start
				}
			}
			if minutes > q.Faculty.MaxWeeklyMinutes {
				return "FACULTY_WORKLOAD"
			}
		}
	}
	days := map[int]bool{}
	for _, m := range proposed {
		days[m.Day] = true
	}
	for day := range days {
		for _, resource := range []string{"faculty", "section"} {
			list := []domain.Meeting{}
			for _, m := range all {
				if m.Day == day && ((resource == "faculty" && m.FacultyID == q.Faculty.ID) || (resource == "section" && m.SectionID == q.Section.ID)) {
					list = append(list, m)
				}
			}
			sort.Slice(list, func(i, j int) bool { return list[i].Start < list[j].Start })
			// Instructors always need a 60-minute lunch break inside the
			// configured window. Sections additionally honor any explicit
			// academic-period lunch setting.
			required := 0
			if resource == "faculty" {
				required = 60
			}
			if e.p.LunchMinutes > required {
				required = e.p.LunchMinutes
			}
			if resource == "section" && e.p.LunchMinutes == 0 {
				required = 0
			}
			if required > 0 {
				cursor := e.p.LunchStart
				best := 0
				for _, m := range list {
					if m.End <= e.p.LunchStart || m.Start >= e.p.LunchEnd {
						continue
					}
					if m.Start-cursor > best {
						best = m.Start - cursor
					}
					if m.End > cursor {
						cursor = m.End
					}
				}
				if e.p.LunchEnd-cursor > best {
					best = e.p.LunchEnd - cursor
				}
				if best < required {
					return "LUNCH_BREAK"
				}
			}
			// Only sections have a consecutive-subject constraint. Faculty
			// may teach consecutive classes subject to lunch and availability.
			if resource == "section" {
				limit := e.p.MaxConsecutive
				if limit <= 0 {
					limit = 3
				}
				count := 0
				end := -10000
				for _, m := range list {
					if m.Start-end >= 60 {
						count = 0
					}
					count++
					if count > limit {
						return "SECTION_CONSECUTIVE_LIMIT"
					}
					if m.End > end {
						end = m.End
					}
				}
			}
		}
	}
	return ""
}
