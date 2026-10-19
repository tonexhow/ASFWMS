// Package domain holds the academic model shared by the API and pure scheduler.
package domain

import (
	"asfwms.local/scheduler/internal/storage"
	"context"
	"encoding/json"
)

type Department struct {
	ID     int    `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active int    `json:"active"`
}
type Program struct {
	ID           int    `json:"id"`
	DepartmentID int    `json:"department_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Active       int    `json:"active"`
}
type Period struct {
	ID             int    `json:"id"`
	Label          string `json:"label"`
	AcademicYear   string `json:"academic_year"`
	Semester       string `json:"semester"`
	Term           string `json:"term"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	Active         int    `json:"active"`
	DayStart       int    `json:"day_start"`
	DayEnd         int    `json:"day_end"`
	LunchStart     int    `json:"lunch_start"`
	LunchEnd       int    `json:"lunch_end"`
	LunchMinutes   int    `json:"lunch_minutes"`
	MaxConsecutive int    `json:"max_consecutive"`
}
type Subject struct {
	ID               int     `json:"id"`
	DepartmentID     int     `json:"department_id"`
	Code             string  `json:"code"`
	Title            string  `json:"title"`
	Units            float64 `json:"units"`
	WeeklyMinutes    int     `json:"weekly_minutes"`
	Priority         string  `json:"priority"`
	RequiredRoom     string  `json:"required_room"`
	Pattern          string  `json:"pattern"`
	AllowedDays      string  `json:"allowed_days"`
	SaturdayFallback int     `json:"saturday_fallback"`
	Active           int     `json:"active"`
}
type SubjectComponent struct {
	ID            int     `json:"id"`
	SubjectID     int     `json:"subject_id"`
	Kind          string  `json:"kind"`
	Units         float64 `json:"units"`
	WeeklyMinutes int     `json:"weekly_minutes"`
	SortOrder     int     `json:"sort_order"`
}
type FacilityRequest struct {
	ID        int `json:"id"`
	SubjectID int `json:"subject_id"`
	RoomID    int `json:"room_id"`
}
type DepartmentCompletion struct {
	DepartmentID int  `json:"department_id"`
	PeriodID     int  `json:"period_id"`
	Complete     bool `json:"complete"`
}
type Curriculum struct {
	ID        int    `json:"id"`
	ProgramID int    `json:"program_id"`
	Name      string `json:"name"`
	YearLevel int    `json:"year_level"` // 0 keeps the legacy multi-year curriculum
	Semester  string `json:"semester"`   // blank for legacy curricula
	Active    int    `json:"active"`
}
type Mapping struct {
	ID             int    `json:"id"`
	CurriculumID   int    `json:"curriculum_id"`
	SubjectID      int    `json:"subject_id"`
	Year           int    `json:"year_level"`
	SourceSemester string `json:"source_semester"`
}
type Section struct {
	ID           int    `json:"id"`
	ProgramID    int    `json:"program_id"`
	CurriculumID int    `json:"curriculum_id"`
	PeriodID     int    `json:"period_id"`
	Code         string `json:"code"`
	Year         int    `json:"year_level"`
	Size         int    `json:"size"`
	AdviserID    int    `json:"adviser_id"`
	Active       int    `json:"active"`
}
type Faculty struct {
	ID                 int    `json:"id"`
	DepartmentID       int    `json:"department_id"`
	Name               string `json:"name"`
	Prefix             string `json:"prefix"`
	FirstName          string `json:"first_name"`
	MiddleName         string `json:"middle_name"`
	LastName           string `json:"last_name"`
	Extension          string `json:"extension"`
	Email              string `json:"email"`
	Employment         string `json:"employment"`
	Consideration      string `json:"consideration"`
	PriorityScheduling int    `json:"priority_scheduling"`
	SectionPolicy      string `json:"section_policy"`
	SourceFacultyID    int    `json:"source_faculty_id"`
	MaxWeeklyMinutes   int    `json:"max_weekly_minutes"`
	Active             int    `json:"active"`
}
type FacultyPreference struct {
	FacultyID       int `json:"faculty_id"`
	FirstStart      int `json:"first_start_minute"`
	FirstEnd        int `json:"first_end_minute"`
	SecondStart     int `json:"second_start_minute"`
	SecondEnd       int `json:"second_end_minute"`
	PreferredRoomID int `json:"preferred_room_id"`
}
type Eligibility struct {
	ID        int `json:"id"`
	FacultyID int `json:"faculty_id"`
	SubjectID int `json:"subject_id"`
}
type Assignment struct {
	ID        int `json:"id"`
	FacultyID int `json:"faculty_id"`
	SubjectID int `json:"subject_id"`
	SectionID int `json:"section_id"`
}
type Window struct {
	ID        int    `json:"id"`
	FacultyID int    `json:"faculty_id"`
	Day       int    `json:"day"`
	Start     int    `json:"start_minute"`
	End       int    `json:"end_minute"`
	Note      string `json:"note"`
}
type Building struct {
	ID           int    `json:"id"`
	DepartmentID int    `json:"department_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Active       int    `json:"active"`
}
type Room struct {
	ID           int    `json:"id"`
	BuildingID   int    `json:"building_id"`
	DepartmentID int    `json:"department_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Capacity     int    `json:"capacity"`
	Start        int    `json:"start_minute"`
	End          int    `json:"end_minute"`
	Active       int    `json:"active"`
}
type Meeting struct {
	ComponentID   int    `json:"component_id,omitempty"`
	ComponentKind string `json:"component_kind,omitempty"`
	ID            int    `json:"id"`
	RunID         int    `json:"run_id"`
	DepartmentID  int    `json:"department_id"`
	PeriodID      int    `json:"period_id"`
	SectionID     int    `json:"section_id"`
	SubjectID     int    `json:"subject_id"`
	FacultyID     int    `json:"faculty_id"`
	RoomID        int    `json:"room_id"`
	Day           int    `json:"day"`
	Start         int    `json:"start_minute"`
	End           int    `json:"end_minute"`
	Borrowed      int    `json:"borrowed"`
	Section       string `json:"section,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Title         string `json:"title,omitempty"`
	Faculty       string `json:"faculty,omitempty"`
	Room          string `json:"room,omitempty"`
	Owner         string `json:"owner,omitempty"`
}
type Snapshot struct {
	Components       []SubjectComponent     `json:"subject_components"`
	FacilityRequests []FacilityRequest      `json:"subject_facility_requests"`
	Completions      []DepartmentCompletion `json:"department_completions"`
	Departments      []Department           `json:"departments"`
	Programs         []Program              `json:"programs"`
	Periods          []Period               `json:"academic_periods"`
	Subjects         []Subject              `json:"subjects"`
	Curricula        []Curriculum           `json:"curricula"`
	Mappings         []Mapping              `json:"curriculum_subject_years"`
	Sections         []Section              `json:"sections"`
	Faculty          []Faculty              `json:"faculty"`
	Eligibility      []Eligibility          `json:"faculty_subjects"`
	Assignments      []Assignment           `json:"teaching_assignments"`
	WorkTimes        []Window               `json:"faculty_work_times"`
	Preferences      []FacultyPreference    `json:"faculty_preferences"`
	Blocks           []Window               `json:"faculty_considerations"`
	Buildings        []Building             `json:"buildings"`
	Rooms            []Room                 `json:"rooms"`
	Meetings         []Meeting              `json:"scheduled_meetings"`
}

var Tables = []string{"departments", "programs", "academic_periods", "subjects", "subject_components", "subject_facility_requests", "curricula", "curriculum_subject_years", "faculty", "sections", "faculty_subjects", "teaching_assignments", "faculty_work_times", "faculty_preferences", "faculty_considerations", "buildings", "rooms", "scheduled_meetings"}

func Load(ctx context.Context, q storage.Querier) (Snapshot, map[string]any, error) {
	data := map[string]any{}
	for _, t := range Tables {
		// Faculty preferences are keyed directly by faculty_id rather than id.
		order := "id"
		if t == "faculty_preferences" {
			order = "faculty_id"
		}
		r, e := storage.ReadRows(ctx, q, "SELECT * FROM "+t+" ORDER BY "+order)
		if e != nil {
			return Snapshot{}, nil, e
		}
		data[t] = r
	}
	completionRows, err := storage.ReadRows(ctx, q, "SELECT department_id,period_id,MAX(CASE WHEN outcome='complete' THEN 1 ELSE 0 END) AS ready FROM generation_runs WHERE status='committed' GROUP BY department_id,period_id")
	if err != nil {
		return Snapshot{}, nil, err
	}
	completions := []DepartmentCompletion{}
	for _, row := range completionRows {
		completions = append(completions, DepartmentCompletion{DepartmentID: Int(row, "department_id"), PeriodID: Int(row, "period_id"), Complete: Int(row, "ready") == 1})
	}
	data["department_completions"] = completions
	b, e := json.Marshal(data)
	if e != nil {
		return Snapshot{}, nil, e
	}
	var s Snapshot
	e = json.Unmarshal(b, &s)
	return s, data, e
}
func (s Snapshot) DepartmentOfSection(id int) int {
	for _, r := range s.Sections {
		if r.ID == id {
			for _, p := range s.Programs {
				if p.ID == r.ProgramID {
					return p.DepartmentID
				}
			}
		}
	}
	return 0
}
func (s Snapshot) Period(id int) (Period, bool) {
	for _, p := range s.Periods {
		if p.ID == id {
			return p, true
		}
	}
	return Period{}, false
}
func (s Snapshot) Eligible(faculty, subject int) bool {
	for _, x := range s.Eligibility {
		if x.FacultyID == faculty && x.SubjectID == subject {
			return true
		}
	}
	return false
}

// SectionRequires checks the reusable section bank against curricula for any
// active semester. The selected academic period is resolved when generating,
// not stored as section ownership. Explicit teaching assignments may therefore
// be prepared before the semester starts.
func (s Snapshot) SectionRequires(section, subject int) bool {
	var sc Section
	found := false
	for _, item := range s.Sections {
		if item.ID == section {
			sc = item
			found = true
			break
		}
	}
	if !found {
		return false
	}
	for _, c := range s.Curricula {
		if c.Active != 1 || c.ProgramID != sc.ProgramID || (c.YearLevel > 0 && c.YearLevel != sc.Year) {
			continue
		}
		for _, m := range s.Mappings {
			if m.CurriculumID == c.ID && m.Year == sc.Year && m.SubjectID == subject {
				return true
			}
		}
	}
	return false
}
