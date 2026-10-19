package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"net/http"
)

// Save subject eligibility as one atomic diff; never strand teaching assignments.
func (a *App) facultySubjects(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		SubjectIDs []int `json:"subject_ids"`
	}
	if err = decode(r, &in); err != nil || len(in.SubjectIDs) > 5000 {
		problem(w, 400, "SUBJECTS", "Send the selected subject IDs.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		exists := false
		for _, f := range s.Faculty {
			if f.ID == id {
				exists = true
			}
		}
		if !exists {
			return errors.New("faculty member not found")
		}
		desired := map[int]bool{}
		current := map[int]domain.Eligibility{}
		for _, x := range s.Eligibility {
			if x.FacultyID == id {
				current[x.SubjectID] = x
			}
		}
		for _, sid := range in.SubjectIDs {
			found := false
			for _, sub := range s.Subjects {
				if sub.ID == sid && (sub.Active == 1 || current[sid].ID != 0) {
					found = true
				}
			}
			if !found {
				return errors.New("choose an existing active subject")
			}
			desired[sid] = true
		}
		for sid, x := range current {
			if !desired[sid] {
				for _, assignment := range s.Assignments {
					if assignment.FacultyID == id && assignment.SubjectID == sid {
						return errors.New("remove this subject's teaching section assignments before removing its eligibility")
					}
				}
				item := map[string]any{"faculty_id": id, "subject_id": sid}
				if e = domain.GuardChanges(s, "faculty_subjects", x.ID, item, nil, true); e != nil {
					return e
				}
				if _, e = q.ExecContext(r.Context(), "DELETE FROM faculty_subjects WHERE id=?", x.ID); e != nil {
					return e
				}
			}
		}
		for sid := range desired {
			if _, ok := current[sid]; !ok {
				item := map[string]any{"faculty_id": id, "subject_id": sid}
				if e = domain.GuardChanges(s, "faculty_subjects", 0, nil, item, false); e != nil {
					return e
				}
				if _, e = q.ExecContext(r.Context(), "INSERT INTO faculty_subjects(faculty_id,subject_id) VALUES(?,?)", id, sid); e != nil {
					return e
				}
			}
		}
		return audit(r.Context(), q, user(r), "subjects", "faculty", id, "Subject eligibility selection updated")
	})
	if err != nil {
		problem(w, 409, "SUBJECTS_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Members are added to the selected existing curriculum, never a new container.
func (a *App) curriculumMembers(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Year       int   `json:"year_level"`
		SubjectIDs []int `json:"subject_ids"`
	}
	if err = decode(r, &in); err != nil || in.Year < 1 || in.Year > 6 || len(in.SubjectIDs) == 0 || len(in.SubjectIDs) > 5000 {
		problem(w, 400, "MEMBERS", "Choose a year level and at least one subject.")
		return
	}
	added := 0
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if e := checkRevision(r, q); e != nil {
			return e
		}
		s, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		var chosen *domain.Curriculum
		for i := range s.Curricula {
			if s.Curricula[i].ID == id && s.Curricula[i].Active == 1 {
				chosen = &s.Curricula[i]
				break
			}
		}
		if chosen == nil {
			return errors.New("select an existing active curriculum")
		}
		if chosen.YearLevel > 0 && chosen.YearLevel != in.Year {
			return errors.New("this curriculum belongs to another year level")
		}
		// Scoped groups can coexist across different semesters. Legacy
		// multi-year groups retain the original one-owner-per-program/year
		// rule with other legacy groups, without blocking newer scopes.
		if chosen.YearLevel == 0 {
			for _, mp := range s.Mappings {
				if mp.CurriculumID == chosen.ID || mp.Year != in.Year {
					continue
				}
				for _, other := range s.Curricula {
					if other.ID == mp.CurriculumID && other.Active == 1 && other.ProgramID == chosen.ProgramID && other.YearLevel == 0 {
						return errors.New("year level already belongs to another legacy curriculum for this program")
					}
				}
			}
		}
		if chosen.YearLevel > 0 {
			for _, other := range s.Curricula {
				if other.ID != id && other.Active == 1 && other.ProgramID == chosen.ProgramID &&
					other.YearLevel == chosen.YearLevel && other.Semester == chosen.Semester {
					return errors.New("this program, year and semester already has another active curriculum")
				}
			}
		}
		seen := map[int]bool{}
		for _, mp := range s.Mappings {
			if mp.CurriculumID == id && mp.Year == in.Year {
				seen[mp.SubjectID] = true
			}
		}
		for _, sid := range in.SubjectIDs {
			if seen[sid] {
				continue
			}
			seen[sid] = true
			valid := false
			for _, sub := range s.Subjects {
				if sub.ID == sid && sub.Active == 1 {
					valid = true
				}
			}
			if !valid {
				return errors.New("choose an existing active subject")
			}
			item := map[string]any{"curriculum_id": id, "year_level": in.Year, "subject_id": sid, "source_semester": chosen.Semester}
			if e = domain.GuardChanges(s, "curriculum_subject_years", 0, nil, item, false); e != nil {
				return e
			}
			if _, e = q.ExecContext(r.Context(), "INSERT INTO curriculum_subject_years(curriculum_id,year_level,subject_id,source_semester) VALUES(?,?,?,?)", id, in.Year, sid, chosen.Semester); e != nil {
				return e
			}
			added++
		}
		return audit(r.Context(), q, user(r), "members", "curricula", id, "Subjects added to existing curriculum/year")
	})
	if err != nil {
		problem(w, 409, "MEMBERS_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]int{"added": added})
}

// subjectFaculty edits the same eligibility relationship as Faculty > Subjects,
// from the academic subject's perspective. No teaching section is implicitly
// assigned. Existing explicit or committed scheduling links cannot be stranded.
func (a *App) subjectFaculty(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var input struct {
		FacultyIDs []int `json:"faculty_ids"`
	}
	if err = decode(r, &input); err != nil || len(input.FacultyIDs) > 5000 {
		problem(w, 400, "FACULTY", "Send faculty_ids as an array of selected instructor IDs.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if err := checkRevision(r, q); err != nil {
			return err
		}
		snapshot, _, err := domain.Load(r.Context(), q)
		if err != nil {
			return err
		}
		found := false
		for _, su := range snapshot.Subjects {
			if su.ID == id && su.Active == 1 {
				found = true
				break
			}
		}
		if !found {
			return errors.New("choose an existing active subject")
		}
		wanted := map[int]bool{}
		current := map[int]domain.Eligibility{}
		for _, x := range snapshot.Eligibility {
			if x.SubjectID == id {
				current[x.FacultyID] = x
			}
		}
		for _, fid := range input.FacultyIDs {
			eligible := false
			for _, f := range snapshot.Faculty {
				if f.ID == fid && (f.Active == 1 || current[fid].ID != 0) {
					if user(r).Role == "scheduler" && f.DepartmentID != user(r).DepartmentID && current[fid].ID == 0 {
						return errors.New("only the faculty owner can add subject eligibility; existing cross-department eligibility remains selectable")
					}
					eligible = true
					break
				}
			}
			if !eligible {
				return errors.New("choose an existing active faculty member")
			}
			wanted[fid] = true
		}
		for fid, eligibility := range current {
			if user(r).Role == "scheduler" && !idHasDepartment(r.Context(), q, "faculty", fid, user(r).DepartmentID) {
				continue
			}
			if wanted[fid] {
				continue
			}
			for _, x := range snapshot.Assignments {
				if x.SubjectID == id && x.FacultyID == fid {
					return errors.New("remove explicit teaching section assignments before unlinking the instructor")
				}
			}
			old := map[string]any{"faculty_id": fid, "subject_id": id}
			if err := domain.GuardChanges(snapshot, "faculty_subjects", eligibility.ID, old, nil, true); err != nil {
				return err
			}
			if _, err := q.ExecContext(r.Context(), "DELETE FROM faculty_subjects WHERE id=?", eligibility.ID); err != nil {
				return err
			}
		}
		for fid := range wanted {
			if _, already := current[fid]; already {
				continue
			}
			added := map[string]any{"faculty_id": fid, "subject_id": id}
			if err := domain.GuardChanges(snapshot, "faculty_subjects", 0, nil, added, false); err != nil {
				return err
			}
			if _, err := q.ExecContext(r.Context(), "INSERT INTO faculty_subjects(faculty_id,subject_id) VALUES(?,?)", fid, id); err != nil {
				return err
			}
		}
		return audit(r.Context(), q, user(r), "eligible-faculty", "subjects", id, "Faculty eligibility selection updated from Academic > Subjects")
	})
	if err != nil {
		problem(w, 409, "FACULTY_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
