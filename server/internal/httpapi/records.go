package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/scheduling"
	"asfwms.local/scheduler/internal/storage"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *App) bootstrap(w http.ResponseWriter, r *http.Request) {
	tx, e := a.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "DATABASE", safeError(e))
		return
	}
	defer tx.Rollback()
	_, data, e := domain.Load(r.Context(), tx)
	if e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	revision, e := storage.Revision(r.Context(), tx)
	if e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	var dataset string
	if e = tx.QueryRowContext(r.Context(), "SELECT value FROM app_meta WHERE key='dataset'").Scan(&dataset); e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	runs, e := storage.ReadRows(r.Context(), tx, "SELECT * FROM generation_runs ORDER BY id DESC LIMIT 100")
	if e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	// Filter the response for departmental schedulers. The engine still uses a global
	// snapshot server-side so foreign-department reservations remain protected.
	u := user(r)
	if u.Role == "scheduler" {
		for _, kind := range domain.Tables {
			input, ok := data[kind].([]map[string]any)
			if !ok {
				continue
			}
			filtered := []map[string]any{}
			for _, item := range input {
				if scopeVisible(r.Context(), tx, u.DepartmentID, kind, item) {
					filtered = append(filtered, item)
				}
			}
			data[kind] = filtered
		}
	}
	// Show only the most recent still-valid preview per department and period.
	// A preview is never mistaken for a committed teaching assignment.
	previewAssignments := []map[string]any{}
	seenScopes := map[string]bool{}
	for _, run := range runs {
		if u.Role == "scheduler" && domain.Int(run, "department_id") != u.DepartmentID {
			continue
		}
		var v scheduling.Result
		if s, ok := run["result_json"].(string); ok {
			_ = json.Unmarshal([]byte(s), &v)
		}
		if status, _ := run["status"].(string); status == "preview" && v.Request.DepartmentID > 0 && v.Request.PeriodID > 0 {
			scope := fmt.Sprintf("%d-%d", v.Request.DepartmentID, v.Request.PeriodID)
			if !seenScopes[scope] {
				seenScopes[scope] = true
				if func() bool {
					switch n := run["revision"].(type) {
					case int64:
						return n == revision
					case int:
						return int64(n) == revision
					default:
						return false
					}
				}() {
					seenMeetings := map[[2]int]bool{}
					for _, meeting := range v.Meetings {
						key := [2]int{meeting.SectionID, meeting.SubjectID}
						if seenMeetings[key] {
							continue
						}
						seenMeetings[key] = true
						previewAssignments = append(previewAssignments, map[string]any{"run_id": run["id"], "faculty_id": meeting.FacultyID, "section_id": meeting.SectionID, "subject_id": meeting.SubjectID, "period_id": meeting.PeriodID})
					}
				}
			}
		}
		delete(run, "result_json")
		run["required"] = v.Required
		run["placed"] = v.Placed
		run["borrowed"] = v.Borrowed
		run["issue_count"] = len(v.Issues)
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "DATABASE", safeError(e))
		return
	}
	if u.Role == "scheduler" {
		filteredRuns := []map[string]any{}
		for _, run := range runs {
			if domain.Int(run, "department_id") == u.DepartmentID {
				filteredRuns = append(filteredRuns, run)
			}
		}
		runs = filteredRuns
	}
	writeJSON(w, 200, map[string]any{"data": data, "revision": revision, "specs": domain.Specs, "runs": runs, "preview_assignments": previewAssignments, "dataset": dataset, "user": user(r)})
}
func checkRevision(r *http.Request, q storage.Querier) error {
	h := r.Header.Get("If-Match")
	if h == "" {
		return nil
	}
	n, e := strconv.ParseInt(strings.Trim(h, "\""), 10, 64)
	if e != nil {
		return errors.New("invalid data revision")
	}
	now, e := storage.Revision(r.Context(), q)
	if e != nil {
		return e
	}
	if n != now {
		return errors.New("records changed in another request; refresh before saving again")
	}
	return nil
}
func validateSnapshot(s domain.Snapshot) error {
	for _, sc := range s.Sections {
		for _, c := range s.Curricula {
			if c.ID != sc.CurriculumID {
				continue
			}
			if c.ProgramID != sc.ProgramID {
				return errors.New("a section would use a curriculum from another program; update its links first")
			}
			if c.YearLevel > 0 && c.YearLevel != sc.Year {
				return errors.New("a section uses a curriculum intended for a different year level")
			}
			for _, period := range s.Periods {
				if period.ID == sc.PeriodID && c.Semester != "" && !domain.SemestersMatch(c.Semester, period.Semester) {
					return errors.New("a section uses a curriculum intended for another semester")
				}
			}
		}
	}
	for _, x := range s.Assignments {
		if !s.SectionRequires(x.SectionID, x.SubjectID) {
			return errors.New("this change would leave a teaching assignment outside its section's curriculum/year; remove that teaching assignment first")
		}
	}
	return nil
}
func (a *App) createRecord(w http.ResponseWriter, r *http.Request) { a.saveRecord(w, r, false) }
func (a *App) updateRecord(w http.ResponseWriter, r *http.Request) { a.saveRecord(w, r, true) }
func (a *App) saveRecord(w http.ResponseWriter, r *http.Request, update bool) {
	kind := r.PathValue("kind")
	if _, ok := domain.Specs[kind]; !ok {
		problem(w, 404, "ENTITY", "Unknown record type.")
		return
	}
	id := 0
	var e error
	if update {
		id, e = idOf(r)
		if e != nil {
			problem(w, 400, "ID", e.Error())
			return
		}
	}
	var in map[string]any
	if e = decode(r, &in); e != nil || in == nil {
		problem(w, 400, "INPUT", "Send a record object with valid fields.")
		return
	}
	var plan *subjectPlan
	if kind == "subjects" {
		plan, e = parseSubjectPlan(in)
		if e != nil {
			problem(w, 400, "SUBJECT_PLAN", e.Error())
			return
		}
	}
	var saved map[string]any
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		old := map[string]any{}
		if update {
			rs, e := storage.ReadRows(r.Context(), q, "SELECT * FROM "+kind+" WHERE id=?", id)
			if e != nil {
				return e
			}
			if len(rs) == 0 {
				return errors.New("record not found")
			}
			old = rs[0]
		}
		// The Section bank is independent of semesters. Old clients may still
		// send explicit nullable curriculum/period fields for compatibility;
		// the standard form and API create term-neutral rows by default.
		if kind == "sections" {
			if !update {
				if _, ok := in["period_id"]; !ok {
					in["period_id"] = nil
				}
				if _, ok := in["curriculum_id"]; !ok {
					in["curriculum_id"] = nil
				}
			} else if domain.Int(old, "period_id") == 0 {
				// Do not reattach a bank section just because the selected term
				// changed elsewhere in the application.
				if _, ok := in["period_id"]; !ok {
					in["period_id"] = nil
				}
				if _, ok := in["curriculum_id"]; !ok {
					in["curriculum_id"] = nil
				}
			}
		}
		if plan != nil && plan.Components != nil {
			applySubjectTotals(in, plan.Components)
		}
		saved, e = domain.Normalize(kind, in, old)
		if e != nil {
			return e
		}
		if update && kind == "subjects" && (plan == nil || plan.Components == nil) {
			for _, component := range s.Components {
				if component.SubjectID != id {
					continue
				}
				if fmt.Sprint(old["units"]) != fmt.Sprint(saved["units"]) || domain.Int(old, "weekly_minutes") != domain.Int(saved, "weekly_minutes") || domain.Str(old, "required_room") != domain.Str(saved, "required_room") {
					return errors.New("edit unit components to change a subject's units or facility type; totals are calculated automatically")
				}
				break
			}
		}
		if kind == "faculty" {
			facultyNumber := domain.Str(saved, "faculty_id_number")
			if facultyNumber != "" {
				for _, member := range s.Faculty {
					if member.ID != id && strings.EqualFold(member.FacultyIDNumber, facultyNumber) {
						return errors.New("faculty ID number is already assigned to another faculty member")
					}
				}
			}
			employment := domain.Str(saved, "employment")
			oldEmployment := domain.Str(old, "employment")
			if employment != oldEmployment || !update {
				if employment != "" && employment != "Full Time" && employment != "Part Time" {
					return errors.New("employment status must be Full Time or Part Time (or left blank)")
				}
			}
		}
		// Changing a curriculum's scope would silently change which sections can
		// use it. Reassignment must happen before such edits, not after the fact.
		if update && kind == "curricula" &&
			(domain.Int(old, "year_level") != domain.Int(saved, "year_level") || domain.Str(old, "semester") != domain.Str(saved, "semester")) {
			for _, m := range s.Mappings {
				if m.CurriculumID == id {
					return errors.New("unlink curriculum subjects before changing its year or semester")
				}
			}
			for _, sec := range s.Sections {
				if sec.CurriculumID == id {
					return errors.New("reassign sections before changing this curriculum's year or semester")
				}
			}
		}
		if kind == "sections" && (!update || domain.Int(old, "program_id") != domain.Int(saved, "program_id") || domain.Int(old, "year_level") != domain.Int(saved, "year_level") || !strings.EqualFold(domain.Str(old, "code"), domain.Str(saved, "code"))) {
			programID := domain.Int(saved, "program_id")
			code := domain.Str(saved, "code")
			if programID == 0 {
				return errors.New("select a valid program for the section")
			}
			for _, existing := range s.Sections {
				if existing.ID != id && existing.ProgramID == programID && existing.Year == domain.Int(saved, "year_level") && strings.EqualFold(existing.Code, code) {
					return fmt.Errorf("%s already exists for this program and year level; choose another section code", code)
				}
			}
		}
		if e = domain.ValidateRelations(s, kind, saved); e != nil {
			return e
		}
		if e = domain.GuardChanges(s, kind, id, old, saved, false); e != nil {
			return e
		}
		if update {
			cols := domain.Columns(kind)
			pairs := []string{}
			args := []any{}
			for _, c := range cols {
				pairs = append(pairs, c+"=?")
				args = append(args, saved[c])
			}
			args = append(args, id)
			_, e = q.ExecContext(r.Context(), "UPDATE "+kind+" SET "+strings.Join(pairs, ",")+" WHERE id=?", args...)
		} else {
			var n int64
			n, e = storage.Insert(r.Context(), q, kind, saved, domain.Columns(kind))
			id = int(n)
		}
		if e != nil {
			return e
		}
		if kind == "subjects" && plan != nil {
			if e = saveSubjectPlan(r.Context(), q, id, plan, s); e != nil {
				return e
			}
		}
		after, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		if e = validateSnapshot(after); e != nil {
			return e
		}
		action := "create"
		if update {
			action = "update"
		}
		return audit(r.Context(), q, user(r), action, kind, id, "Record saved")
	})
	if e != nil {
		problem(w, 409, "SAVE_BLOCKED", safeError(e))
		return
	}
	saved["id"] = id
	writeJSON(w, 200, saved)
}
func (a *App) deleteRecord(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if _, ok := domain.Specs[kind]; !ok {
		problem(w, 404, "ENTITY", "Unknown record type.")
		return
	}
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Confirm     string `json:"confirm"`
		Strategy    string `json:"strategy"`
		Fingerprint string `json:"fingerprint"`
		Revision    int64  `json:"revision"`
	}
	if err = decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	strategy := "single"
	if in.Strategy == "preserve" {
		strategy = "preserve"
	} else if in.Strategy == "linked" {
		strategy = "linked"
	} else if in.Strategy == "unlink" {
		strategy = "unlink"
	}
	needed := "DELETE"
	if strategy == "preserve" {
		needed = "DELETE AND UNLINK"
	} else if strategy == "linked" {
		needed = "DELETE LINKED"
	} else if strategy == "unlink" {
		needed = "UNLINK"
	}
	if in.Confirm != needed {
		problem(w, 400, "CONFIRM", "Type "+needed+" to confirm.")
		return
	}
	preview, err := buildDeletePlan(r.Context(), a.Store.DB, kind, id)
	if err != nil {
		problem(w, 404, "DELETE", safeError(err))
		return
	}
	if in.Revision == 0 && strategy == "single" {
		in.Revision = preview.Preview.Revision
	} // Legacy simple deletion requests. Unlink always requires an explicit reviewed revision.
	backup, result, err := a.applyBusinessDeletion(r.Context(), kind, id, strategy, in.Fingerprint, in.Revision, user(r))
	if err != nil {
		problem(w, 409, "DELETE_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "backup": backup, "deleted": result.Total, "detached": len(result.Detaches), "unlinked": strategy == "unlink"})
}
func (a *App) assign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FacultyID  int   `json:"faculty_id"`
		SubjectID  int   `json:"subject_id"`
		SectionIDs []int `json:"section_ids"`
	}
	if e := decode(r, &in); e != nil || len(in.SectionIDs) == 0 || len(in.SectionIDs) > 1000 {
		problem(w, 400, "ASSIGNMENT", "Choose a faculty member, subject and at least one section.")
		return
	}
	added := 0
	e := a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		seen := map[int]bool{}
		for _, sid := range in.SectionIDs {
			if seen[sid] {
				continue
			}
			seen[sid] = true
			item := map[string]any{"faculty_id": in.FacultyID, "subject_id": in.SubjectID, "section_id": sid}
			if e = domain.ValidateRelations(s, "teaching_assignments", item); e != nil {
				return e
			}
			exists := false
			for _, x := range s.Assignments {
				if x.SectionID == sid && x.SubjectID == in.SubjectID {
					if x.FacultyID != in.FacultyID {
						return fmt.Errorf("section %d already has another instructor for this subject; remove that assignment explicitly first", sid)
					}
					exists = true
				}
			}
			if exists {
				continue
			}
			if e = domain.GuardChanges(s, "teaching_assignments", 0, nil, item, false); e != nil {
				return e
			}
			if _, e = storage.Insert(r.Context(), q, "teaching_assignments", item, domain.Columns("teaching_assignments")); e != nil {
				return e
			}
			added++
		}
		return audit(r.Context(), q, user(r), "assign", "faculty", in.FacultyID, fmt.Sprintf("%d subject-section assignments added", added))
	})
	if e != nil {
		problem(w, 409, "ASSIGNMENT_BLOCKED", safeError(e))
		return
	}
	writeJSON(w, 200, map[string]int{"added": added})
}

// reassign replaces the explicit subject-section instructor atomically. It is
// deliberately separate from subject eligibility. A committed schedule blocks
// this write until the affected reservation is explicitly released.
func (a *App) reassign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FacultyID int    `json:"faculty_id"`
		SubjectID int    `json:"subject_id"`
		SectionID int    `json:"section_id"`
		Confirm   string `json:"confirm"`
	}
	if e := decode(r, &in); e != nil || in.FacultyID <= 0 || in.SubjectID <= 0 || in.SectionID <= 0 || in.Confirm != "REASSIGN" {
		problem(w, 400, "REASSIGN", "Select an eligible faculty member, a subject and a section; confirm REASSIGN.")
		return
	}
	changed := false
	err := a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		snap, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		desired := map[string]any{"faculty_id": in.FacultyID, "subject_id": in.SubjectID, "section_id": in.SectionID}
		if e = domain.ValidateRelations(snap, "teaching_assignments", desired); e != nil {
			return e
		}
		var existing domain.Assignment
		for _, x := range snap.Assignments {
			if x.SubjectID == in.SubjectID && x.SectionID == in.SectionID {
				existing = x
				break
			}
		}
		if existing.ID != 0 && existing.FacultyID == in.FacultyID {
			return nil
		}
		if existing.ID != 0 {
			old := map[string]any{"faculty_id": existing.FacultyID, "subject_id": existing.SubjectID, "section_id": existing.SectionID}
			if e = domain.GuardChanges(snap, "teaching_assignments", existing.ID, old, desired, false); e != nil {
				return e
			}
			_, e = q.ExecContext(r.Context(), "UPDATE teaching_assignments SET faculty_id=? WHERE id=?", in.FacultyID, existing.ID)
		} else {
			if e = domain.GuardChanges(snap, "teaching_assignments", 0, nil, desired, false); e != nil {
				return e
			}
			_, e = q.ExecContext(r.Context(), "INSERT INTO teaching_assignments(faculty_id,subject_id,section_id) VALUES(?,?,?)", in.FacultyID, in.SubjectID, in.SectionID)
		}
		if e != nil {
			return e
		}
		changed = true
		return audit(r.Context(), q, user(r), "reassign", "teaching_assignments", in.SectionID, "Explicit instructor for subject-section changed")
	})
	if err != nil {
		problem(w, 409, "REASSIGN_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true, "changed": changed})
}
