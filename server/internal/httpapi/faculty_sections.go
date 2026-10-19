package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"errors"
	"fmt"
	"net/http"
)

// One transaction for every subject's section selections plus the policy. Child
// pickers only edit a draft; cancelling the outer modal cannot leave partial saves.
func (a *App) facultySectionSelections(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Policy   string `json:"policy"`
		Subjects []struct {
			SubjectID  int   `json:"subject_id"`
			SectionIDs []int `json:"section_ids"`
		} `json:"subjects"`
	}
	if decode(r, &in) != nil || (in.Policy != "priority" && in.Policy != "exclusive") || len(in.Subjects) > 5000 {
		problem(w, 400, "SECTIONS", "Choose Priority or Strict and valid subject-section selections.")
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
		f := getFaculty(s, id)
		if f.ID == 0 {
			return errors.New("faculty member not found")
		}
		desired := map[[2]int]bool{}
		current := map[[2]int]domain.Assignment{}
		all := map[[2]int]domain.Assignment{}
		for _, x := range s.Assignments {
			k := [2]int{x.SectionID, x.SubjectID}
			all[k] = x
			if x.FacultyID == id {
				current[k] = x
				if user(r).Role == "scheduler" && s.DepartmentOfSection(x.SectionID) != user(r).DepartmentID {
					desired[k] = true
				}
			}
		}
		for _, sub := range in.Subjects {
			if !s.Eligible(id, sub.SubjectID) {
				return errors.New("select a subject from this faculty member's eligibility list")
			}
			if len(sub.SectionIDs) > 5000 {
				return errors.New("too many section selections")
			}
			for _, section := range sub.SectionIDs {
				k := [2]int{section, sub.SubjectID}
				if !s.SectionRequires(section, sub.SubjectID) {
					return errors.New("a selected section does not require this subject in its curriculum/year")
				}
				if user(r).Role == "scheduler" && s.DepartmentOfSection(section) != user(r).DepartmentID {
					if current[k].ID == 0 {
						return errors.New("select sections owned by your department")
					}
				}
				if x, ok := all[k]; ok && x.FacultyID != id {
					return fmt.Errorf("section %s is already assigned to another instructor for this subject", getSection(s, section).Code)
				}
				desired[k] = true
			}
		}
		if in.Policy == "exclusive" && len(desired) == 0 {
			return errors.New("Strict requires at least one selected section")
		}
		for k, x := range current {
			if desired[k] {
				continue
			}
			old := map[string]any{"faculty_id": id, "section_id": x.SectionID, "subject_id": x.SubjectID}
			if e = domain.GuardChanges(s, "teaching_assignments", x.ID, old, nil, true); e != nil {
				return e
			}
			if _, e = q.ExecContext(r.Context(), "DELETE FROM teaching_assignments WHERE id=?", x.ID); e != nil {
				return e
			}
		}
		for k := range desired {
			if current[k].ID != 0 {
				continue
			}
			item := map[string]any{"faculty_id": id, "section_id": k[0], "subject_id": k[1]}
			if e = domain.ValidateRelations(s, "teaching_assignments", item); e != nil {
				return e
			}
			if e = domain.GuardChanges(s, "teaching_assignments", 0, nil, item, false); e != nil {
				return e
			}
			if _, e = q.ExecContext(r.Context(), "INSERT INTO teaching_assignments(faculty_id,section_id,subject_id) VALUES(?,?,?)", id, k[0], k[1]); e != nil {
				return e
			}
		}
		if _, e = q.ExecContext(r.Context(), "UPDATE faculty SET section_policy=? WHERE id=?", in.Policy, id); e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "sections", "faculty", id, "Section selections and handling policy saved atomically")
	})
	if err != nil {
		problem(w, 409, "SECTIONS_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
