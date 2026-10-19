package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Field struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  any      `json:"default,omitempty"`
	Ref      string   `json:"ref,omitempty"`
	Options  []string `json:"options,omitempty"`
	Min      float64  `json:"min,omitempty"`
	Max      float64  `json:"max,omitempty"`
	Help     string   `json:"help,omitempty"`
}
type Spec struct {
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

func text(n, l string, required bool) Field {
	return Field{Name: n, Label: l, Type: "text", Required: required, Default: ""}
}
func ref(n, l, t string, required bool) Field {
	return Field{Name: n, Label: l, Type: "relation", Ref: t, Required: required}
}
func num(n, l string, d, min, max int) Field {
	return Field{Name: n, Label: l, Type: "integer", Required: true, Default: d, Min: float64(min), Max: float64(max)}
}
func clock(n, l string, d int) Field { f := num(n, l, d, 0, 1440); f.Type = "time"; return f }
func choice(n, l, d string, opts ...string) Field {
	return Field{Name: n, Label: l, Type: "select", Required: true, Default: d, Options: opts}
}
func active() Field {
	return Field{Name: "active", Label: "Active", Type: "checkbox", Required: true, Default: 1, Min: 0, Max: 1}
}

var Specs = map[string]Spec{
	"departments":              {"Department", []Field{text("code", "Code", true), text("name", "Department name", true), active()}},
	"programs":                 {"Program", []Field{ref("department_id", "Department", "departments", true), text("code", "Code", true), text("name", "Program name", true), active()}},
	"academic_periods":         {"Academic period", []Field{text("label", "Period label (optional; automatic when blank)", false), text("academic_year", "Academic year (YYYY-YYYY)", true), text("semester", "Semester", true), text("term", "Term / session (optional)", false), {Name: "start_date", Label: "Start date (optional)", Type: "date", Required: false, Default: ""}, {Name: "end_date", Label: "End date (optional)", Type: "date", Required: false, Default: ""}, active(), clock("day_start", "Day starts", 420), clock("day_end", "Day ends", 1140), clock("lunch_start", "Lunch window starts", 660), clock("lunch_end", "Lunch window ends", 840), num("lunch_minutes", "Minimum lunch break (minutes)", 60, 60, 180), num("max_consecutive", "Section maximum consecutive subjects", 3, 3, 3)}},
	"subjects":                 {"Subject", []Field{ref("department_id", "Department", "departments", false), text("code", "Subject code", true), text("title", "Description / title", true), {Name: "units", Label: "Units", Type: "number", Default: 3, Min: 0, Max: 60}, num("weekly_minutes", "Weekly contact minutes", 180, 15, 3600), choice("priority", "Placement priority", "major", "major", "minor"), choice("required_room", "Required facility", "any", "any", "classroom", "laboratory", "field"), choice("pattern", "Meeting pattern", "auto", "auto", "mwf", "tth", "mw", "mf", "wf", "sat", "sun", "single", "flex"), {Name: "allowed_days", Label: "Meeting days", Type: "weekdays", Required: true, Default: "1,2,3,4,5", Help: "Only checked days are regular meeting days. Select Sunday only or Saturday only when required."}, {Name: "saturday_fallback", Label: "Allow Saturday fallback when selected days are full", Type: "checkbox", Default: 0, Help: "Optional, usually for minor subjects. Requires Saturday faculty availability; never enables Sunday."}, active()}},
	"curricula":                {"Curriculum", []Field{ref("program_id", "Program", "programs", true), text("name", "Curriculum name", true), {Name: "year_level", Label: "Curriculum year (0 = legacy / all years)", Type: "integer", Default: 0, Min: 0, Max: 6}, {Name: "semester", Label: "Curriculum semester (blank = legacy)", Type: "text", Default: ""}, active()}},
	"curriculum_subject_years": {"Curriculum subject", []Field{ref("curriculum_id", "Curriculum", "curricula", true), ref("subject_id", "Subject", "subjects", true), num("year_level", "Year level", 1, 1, 6), text("source_semester", "Reference semester (informational only)", false)}},
	"sections":                 {"Section", []Field{ref("program_id", "Program", "programs", true), ref("curriculum_id", "Legacy curriculum link (optional)", "curricula", false), ref("period_id", "Legacy academic period (optional)", "academic_periods", false), text("code", "Section code", true), num("year_level", "Year level", 1, 1, 6), num("size", "Expected students (0 = unspecified)", 50, 0, 1000), ref("adviser_id", "Adviser (independent of teaching)", "faculty", false), active()}},
	"faculty":                  {"Faculty member", []Field{text("name", "Display name", true), text("prefix", "Pre (optional)", false), text("first_name", "First name", false), text("middle_name", "Middle name (optional)", false), text("last_name", "Last name", false), text("extension", "Extension (optional)", false), text("email", "Email (optional)", false), ref("department_id", "Department", "departments", false), text("employment", "Employment (optional)", false), choice("section_policy", "Section policy", "priority", "priority", "exclusive"), {Name: "priority_scheduling", Label: "Consideration: schedule this faculty first", Type: "checkbox", Default: 0, Help: "Major subjects are placed first; faculty consideration leads within each major/minor group. Working hours remain hard limits."}, {Name: "consideration", Label: "Notes (not enforced by the scheduler)", Type: "textarea", Default: ""}, num("max_weekly_minutes", "Maximum weekly teaching minutes (0 = uncapped)", 0, 0, 5040), active()}},
	"faculty_subjects":         {"Eligible subject", []Field{ref("faculty_id", "Faculty", "faculty", true), ref("subject_id", "Subject", "subjects", true)}},
	"teaching_assignments":     {"Teaching assignment", []Field{ref("faculty_id", "Instructor", "faculty", true), ref("subject_id", "Subject", "subjects", true), ref("section_id", "Section", "sections", true)}},
	"faculty_work_times":       {"Working day", []Field{ref("faculty_id", "Faculty", "faculty", true), num("day", "Day (1 = Monday, 7 = Sunday)", 1, 1, 7), clock("start_minute", "Starts", 420), clock("end_minute", "Ends", 1140)}},
	"faculty_considerations":   {"Unavailable window", []Field{ref("faculty_id", "Faculty", "faculty", true), num("day", "Day (1 = Monday, 7 = Sunday)", 1, 1, 7), clock("start_minute", "Starts", 720), clock("end_minute", "Ends", 780), text("note", "Reason (optional)", false)}},
	"buildings":                {"Building / field area", []Field{text("code", "Code", true), text("name", "Name", true), choice("kind", "Type", "building", "building", "field"), ref("department_id", "Department priority", "departments", false), active()}},
	"rooms":                    {"Room / teaching area", []Field{ref("building_id", "Building / field area", "buildings", true), ref("department_id", "Department owner (empty = shared)", "departments", false), text("code", "Room code", true), text("name", "Room name", true), choice("kind", "Room type", "classroom", "classroom", "laboratory", "field"), num("capacity", "Capacity (0 = unspecified)", 50, 0, 1000), clock("start_minute", "Available from", 420), clock("end_minute", "Available until", 1140), active()}},
}

func Columns(kind string) []string {
	out := []string{}
	for _, f := range Specs[kind].Fields {
		out = append(out, f.Name)
	}
	return out
}
func Int(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
func Str(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func Normalize(kind string, input, existing map[string]any) (map[string]any, error) {
	if kind == "faculty" {
		var err error
		input, err = normalizeFacultyName(input, existing)
		if err != nil {
			return nil, err
		}
	}
	spec, ok := Specs[kind]
	if !ok {
		return nil, errors.New("unknown record type")
	}
	allowed := map[string]bool{}
	for _, f := range spec.Fields {
		allowed[f.Name] = true
	}
	for k := range input {
		if !allowed[k] {
			return nil, fmt.Errorf("unknown field: %s", k)
		}
	}
	out := map[string]any{}
	for _, f := range spec.Fields {
		v, ok := input[f.Name]
		if !ok {
			v, ok = existing[f.Name]
		}
		if !ok {
			v = f.Default
		}
		switch f.Type {
		case "integer", "number", "time", "checkbox", "relation":
			if v == nil && f.Type == "relation" && !f.Required {
				out[f.Name] = nil
				continue
			}
			var n float64
			switch x := v.(type) {
			case float64:
				n = x
			case int:
				n = float64(x)
			case int64:
				n = float64(x)
			case nil:
				return nil, fmt.Errorf("%s is required", f.Label)
			default:
				return nil, fmt.Errorf("%s must be numeric", f.Label)
			}
			if math.IsInf(n, 0) || math.IsNaN(n) || (f.Type != "number" && n != math.Trunc(n)) {
				return nil, fmt.Errorf("%s must be a valid %s", f.Label, f.Type)
			}
			if f.Type == "relation" {
				if n == 0 && !f.Required {
					out[f.Name] = nil
					continue
				}
				if n < 1 || n > 2147483647 {
					return nil, fmt.Errorf("choose %s", f.Label)
				}
			} else if f.Type == "checkbox" {
				if n != 0 && n != 1 {
					return nil, fmt.Errorf("%s must be on or off", f.Label)
				}
			} else if n < f.Min || n > f.Max {
				return nil, fmt.Errorf("%s must be between %g and %g", f.Label, f.Min, f.Max)
			}
			if f.Type == "number" {
				out[f.Name] = n
			} else {
				out[f.Name] = int64(n)
			}
		default:
			if v == nil {
				v = ""
			}
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be text", f.Label)
			}
			s = strings.TrimSpace(s)
			if f.Required && s == "" {
				return nil, fmt.Errorf("%s is required", f.Label)
			}
			max := 240
			if f.Type == "textarea" {
				max = 2000
			}
			if len([]rune(s)) > max {
				return nil, fmt.Errorf("%s is too long (maximum %d characters)", f.Label, max)
			}
			if f.Type == "select" {
				ok = false
				for _, x := range f.Options {
					if s == x {
						ok = true
					}
				}
				if !ok {
					return nil, fmt.Errorf("invalid %s", f.Label)
				}
			}
			if f.Type == "date" {
				if s != "" {
					if _, e := time.Parse("2006-01-02", s); e != nil {
						return nil, fmt.Errorf("%s must be a valid calendar date", f.Label)
					}
				}
			}
			out[f.Name] = s
		}
	}
	if _, ok := out["start_minute"]; ok {
		if Int(out, "end_minute") <= Int(out, "start_minute") {
			return nil, errors.New("end time must be after start time")
		}
	}
	if kind == "subjects" {
		normalized, err := NormalizeDays(Str(out, "allowed_days"))
		if err != nil {
			return nil, err
		}
		out["allowed_days"] = normalized
		if Int(out, "saturday_fallback") == 1 && ContainsDay(normalized, 6) {
			return nil, errors.New("Saturday is already a regular meeting day; turn off fallback or uncheck Saturday")
		}
	}
	if kind == "subjects" && Int(out, "weekly_minutes")%15 != 0 {
		return nil, errors.New("weekly contact minutes must be a multiple of 15; the generator never silently rounds contact hours")
	}
	if kind == "curricula" {
		year, semester := Int(out, "year_level"), Str(out, "semester")
		allowed := map[string]bool{"": true, "1st Semester": true, "2nd Semester": true, "Summer": true, "Others": true}
		if !allowed[semester] {
			return nil, errors.New("choose a valid semester")
		}
		if (year == 0) != (semester == "") {
			return nil, errors.New("choose both the curriculum year level and semester, or leave both unset for a legacy curriculum")
		}
	}
	if kind == "academic_periods" {
		academicYear := Str(out, "academic_year")
		if len(academicYear) != 9 || academicYear[4] != '-' {
			return nil, errors.New("academic year must use consecutive years, for example 2026-2027")
		}
		first, err := time.Parse("2006", academicYear[:4])
		if err != nil {
			return nil, errors.New("academic year must start with a four-digit year")
		}
		second, err := time.Parse("2006", academicYear[5:])
		if err != nil || second.Year() != first.Year()+1 {
			return nil, errors.New("academic year must contain two consecutive years (2026-2027)")
		}
		if Str(out, "semester") == "" {
			return nil, errors.New("choose a semester")
		}
		if Str(out, "label") == "" {
			out["label"] = academicYear + " / " + Str(out, "semester")
		}
		if Str(out, "start_date") != "" && Str(out, "end_date") != "" && Str(out, "end_date") < Str(out, "start_date") {
			return nil, errors.New("period end must not precede its start")
		}
		if Int(out, "day_start") >= Int(out, "day_end") {
			return nil, errors.New("day end must follow day start")
		}
		if Int(out, "lunch_start") >= Int(out, "lunch_end") || Int(out, "lunch_minutes") > Int(out, "lunch_end")-Int(out, "lunch_start") {
			return nil, errors.New("invalid lunch window or duration")
		}
		if Int(out, "lunch_minutes") > 0 && (Int(out, "lunch_start") < Int(out, "day_start") || Int(out, "lunch_end") > Int(out, "day_end")) {
			return nil, errors.New("enabled lunch window must be inside the school day")
		}
	}
	return out, nil
}

// SemestersMatch accepts the common names used in imported academic periods.
// Curriculum scope storage remains canonical (1st Semester/2nd Semester/etc.).
func SemestersMatch(a, b string) bool {
	normalized := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.Join(strings.Fields(value), " ")
		switch value {
		case "1st semester", "first semester", "first", "1st", "semester 1", "1":
			return "first"
		case "2nd semester", "second semester", "second", "2nd", "semester 2", "2":
			return "second"
		case "summer semester", "summer term", "summer":
			return "summer"
		case "other", "others":
			return "others"
		}
		return value
	}
	return normalized(a) == normalized(b)
}

// ValidateRelations prevents a teaching assignment from becoming a second adviser path.
func ValidateRelations(s Snapshot, kind string, r map[string]any) error {
	if kind == "sections" {
		for _, c := range s.Curricula {
			if c.ID != Int(r, "curriculum_id") {
				continue
			}
			if c.ProgramID != Int(r, "program_id") {
				return errors.New("the selected curriculum belongs to a different program")
			}
			if c.YearLevel > 0 && c.YearLevel != Int(r, "year_level") {
				return errors.New("the selected curriculum is for a different year level")
			}
			if c.Semester != "" {
				for _, p := range s.Periods {
					if p.ID == Int(r, "period_id") && !SemestersMatch(p.Semester, c.Semester) {
						return errors.New("the selected curriculum semester does not match the section academic period")
					}
				}
			}
		}
	}
	if kind == "curriculum_subject_years" {
		cid, year := Int(r, "curriculum_id"), Int(r, "year_level")
		var current *Curriculum
		for i := range s.Curricula {
			if s.Curricula[i].ID == cid {
				current = &s.Curricula[i]
				break
			}
		}
		if current != nil {
			if current.YearLevel > 0 && current.YearLevel != year {
				return errors.New("subjects must be mapped to this curriculum's selected year level")
			}
			if current.Semester != "" && current.Semester != Str(r, "source_semester") {
				return errors.New("curriculum subject's semester must match the selected curriculum semester")
			}
		}
		// Preserve legacy ownership checks for legacy-vs-legacy mappings,
		// while scoped offerings can reuse subjects in other semesters.
		if current != nil && current.YearLevel == 0 {
			for _, mp := range s.Mappings {
				if mp.CurriculumID == cid || mp.Year != year {
					continue
				}
				for _, other := range s.Curricula {
					if other.ID == mp.CurriculumID && other.ProgramID == current.ProgramID && other.Active == 1 && other.YearLevel == 0 {
						return errors.New("year level belongs to another legacy curriculum in this program")
					}
				}
			}
		}
		if current != nil && current.YearLevel > 0 {
			for _, other := range s.Curricula {
				if other.ID != cid && other.Active == 1 && other.ProgramID == current.ProgramID && other.YearLevel == year && other.Semester == current.Semester {
					return errors.New("a curriculum already exists for this program, year and semester")
				}
			}
		}
	}
	if kind == "teaching_assignments" {
		if !s.Eligible(Int(r, "faculty_id"), Int(r, "subject_id")) {
			return errors.New("add this subject to the faculty member's eligible subjects first")
		}
		if !s.SectionRequires(Int(r, "section_id"), Int(r, "subject_id")) {
			return errors.New("this subject is not in the section's curriculum and year")
		}
		for _, f := range s.Faculty {
			if f.ID == Int(r, "faculty_id") && f.Active == 0 {
				return errors.New("an inactive faculty member cannot receive a teaching assignment")
			}
		}
		for _, v := range s.Sections {
			if v.ID == Int(r, "section_id") && v.Active == 0 {
				return errors.New("an inactive section cannot receive a teaching assignment")
			}
		}
	}
	return nil
}

// GuardChanges blocks schedule-affecting edits until their committed schedule is explicitly released.
func GuardChanges(s Snapshot, kind string, id int, old, new map[string]any, deleting bool) error {
	safe := map[string]bool{"name": true, "code": true, "title": true, "employment": true, "consideration": true, "priority_scheduling": true, "adviser_id": true, "source_semester": true, "label": true, "academic_year": true, "semester": true, "term": true}
	impact := deleting || id == 0
	for k, v := range new {
		if !safe[k] && fmt.Sprint(v) != fmt.Sprint(old[k]) {
			impact = true
		}
	}
	if !impact {
		return nil
	}
	rows := []map[string]any{old, new}
	for _, m := range s.Meetings {
		hit := false
		for _, r := range rows {
			switch kind {
			case "subjects":
				hit = hit || m.SubjectID == id
			case "sections":
				hit = hit || m.SectionID == id
			case "faculty":
				hit = hit || m.FacultyID == id
			case "rooms":
				hit = hit || m.RoomID == id
			case "academic_periods":
				hit = hit || m.PeriodID == id
			case "departments":
				hit = hit || m.DepartmentID == id
				for _, rm := range s.Rooms {
					if rm.ID == m.RoomID && rm.DepartmentID == id {
						hit = true
					}
				}
			case "programs":
				for _, sc := range s.Sections {
					if sc.ID == m.SectionID && sc.ProgramID == id {
						hit = true
					}
				}
			case "curricula":
				for _, sc := range s.Sections {
					if sc.ID == m.SectionID && sc.CurriculumID == id {
						hit = true
					}
				}
			case "curriculum_subject_years":
				for _, sc := range s.Sections {
					if sc.ID == m.SectionID && sc.CurriculumID == Int(r, "curriculum_id") && sc.Year == Int(r, "year_level") {
						hit = true
					}
				}
			case "faculty_work_times", "faculty_considerations":
				hit = hit || m.FacultyID == Int(r, "faculty_id")
			case "faculty_subjects":
				hit = hit || (m.FacultyID == Int(r, "faculty_id") && m.SubjectID == Int(r, "subject_id"))
			case "teaching_assignments":
				hit = hit || (m.SectionID == Int(r, "section_id") && m.SubjectID == Int(r, "subject_id"))
			case "buildings":
				for _, rm := range s.Rooms {
					if rm.ID == m.RoomID && rm.BuildingID == id {
						hit = true
					}
				}
			}
		}
		if hit {
			return errors.New("this change could invalidate a committed schedule; release the affected department schedule first (labels and advisers can still be edited)")
		}
	}
	return nil
}
