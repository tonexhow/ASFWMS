package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func roleAdmin(u User) bool { return u.Role == "developer" || u.Role == "administrator" }
func deny(w http.ResponseWriter) bool {
	problem(w, 403, "SCOPE", "Your account does not have access to this department or operation.")
	return false
}
func (a *App) authorizeRoute(w http.ResponseWriter, r *http.Request, u User) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/developer/") && u.Role != "developer" {
		return deny(w)
	}
	if strings.HasPrefix(path, "/api/access/") && !roleAdmin(u) {
		return deny(w)
	}
	if (path == "/api/backup" || path == "/api/seed") && u.Role != "developer" {
		return deny(w)
	}
	if u.Role != "scheduler" {
		return true
	}
	if u.DepartmentID <= 0 {
		return deny(w)
	}
	if r.Method == "GET" && (path == "/api/bootstrap" || path == "/api/me" || path == "/api/manifest" || strings.HasPrefix(path, "/api/runs/") || path == "/api/timetable/pdf" || path == "/api/timetable/zip") {
		if strings.HasPrefix(path, "/api/runs/") {
			return a.runScope(w, r, u)
		}
		return true
	}
	if path == "/api/waits" || strings.HasPrefix(path, "/api/waits/") {
		return true
	} // handlers enforce department ownership
	if path == "/api/timetable/faculty/email" {
		// Handler rechecks both faculty ownership and committed meeting scope.
		return true
	}
	if path == "/api/account/password" || path == "/api/account/reset-request" || path == "/api/logout" {
		return true
	}
	if strings.HasPrefix(path, "/api/runs/") {
		return a.runScope(w, r, u)
	}
	// Prevent schedulers from altering global records outside their department.
	if strings.HasPrefix(path, "/api/records/") {
		kind := r.PathValue("kind")
		switch kind {
		case "departments", "academic_periods", "curricula":
			return deny(w)
		}
		if r.Method != "POST" {
			id, _ := strconv.Atoi(r.PathValue("id"))
			if id < 1 {
				return deny(w)
			}
			old, err := storage.ReadRows(r.Context(), a.Store.DB, "SELECT * FROM "+safeKind(kind)+" WHERE id=?", id)
			if err != nil || len(old) == 0 || !ownsRow(r.Context(), a.Store.DB, u.DepartmentID, kind, old[0]) {
				return deny(w)
			}
		}
		// A dependency preview is a read-only operation; it does not need a JSON body.
		if r.Method == "GET" && strings.HasSuffix(path, "/dependencies") {
			return true
		}
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
			if err != nil {
				return deny(w)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var data map[string]any
			if json.Unmarshal(body, &data) != nil {
				return deny(w)
			}
			dept := numberField(data, "department_id")
			switch kind {
			case "programs", "faculty", "subjects", "buildings", "rooms":
				// A departmental Scheduler must not create or transfer records into a
				// shared or different department. Admins retain cross-scope access.
				_, suppliedDepartment := data["department_id"]
				if (r.Method == "POST" || suppliedDepartment) && dept != u.DepartmentID {
					return deny(w)
				}
			case "sections":
				if pid := numberField(data, "program_id"); pid > 0 && !idHasDepartment(r.Context(), a.Store.DB, "programs", pid, u.DepartmentID) {
					return deny(w)
				}
			case "curriculum_subject_years":
				return deny(w)
			case "teaching_assignments":
				if sec := numberField(data, "section_id"); sec > 0 && !idHasDepartment(r.Context(), a.Store.DB, "sections", sec, u.DepartmentID) {
					return deny(w)
				}
			case "faculty_subjects", "faculty_work_times", "faculty_considerations":
				if fid := numberField(data, "faculty_id"); fid > 0 && !idHasDepartment(r.Context(), a.Store.DB, "faculty", fid, u.DepartmentID) {
					return deny(w)
				}
			}
		}
		return true
	}
	if path == "/api/generate" || path == "/api/generate/stream" || path == "/api/readiness" {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		r.Body = io.NopCloser(bytes.NewReader(b))
		var v struct {
			DepartmentID int `json:"department_id"`
		}
		if json.Unmarshal(b, &v) != nil || v.DepartmentID != u.DepartmentID {
			return deny(w)
		}
		return true
	}
	if strings.HasPrefix(path, "/api/faculty/") {
		return idHasDepartment(r.Context(), a.Store.DB, "faculty", getIDFromPath(path, 3), u.DepartmentID) || deny(w)
	}
	if strings.HasPrefix(path, "/api/subjects/") {
		if strings.HasSuffix(path, "/faculty") {
			return true
		}
		return idHasDepartment(r.Context(), a.Store.DB, "subjects", getIDFromPath(path, 3), u.DepartmentID) || deny(w)
	}
	if strings.HasPrefix(path, "/api/curricula/") {
		return deny(w)
	}
	if path == "/api/assignments" || path == "/api/assignments/reassign" {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		r.Body = io.NopCloser(bytes.NewReader(b))
		var v struct {
			FacultyID  int   `json:"faculty_id"`
			SubjectID  int   `json:"subject_id"`
			SectionID  int   `json:"section_id"`
			SectionIDs []int `json:"section_ids"`
		}
		if json.Unmarshal(b, &v) != nil {
			return deny(w)
		}
		if v.SectionID == 0 && len(v.SectionIDs) == 0 {
			return deny(w)
		}
		if v.SectionID > 0 && !idHasDepartment(r.Context(), a.Store.DB, "sections", v.SectionID, u.DepartmentID) {
			return deny(w)
		}
		for _, id := range v.SectionIDs {
			if !idHasDepartment(r.Context(), a.Store.DB, "sections", id, u.DepartmentID) {
				return deny(w)
			}
		}
		return true
	}
	return deny(w)
}
func numberField(m map[string]any, k string) int { f, _ := m[k].(float64); return int(f) }
func getIDFromPath(path string, n int) int {
	s := strings.Split(path, "/")
	if len(s) > n {
		v, _ := strconv.Atoi(s[n])
		return v
	}
	return 0
}
func safeKind(kind string) string {
	if _, ok := domain.Specs[kind]; ok {
		return kind
	}
	return "departments"
}
func (a *App) runScope(w http.ResponseWriter, r *http.Request, u User) bool {
	id := getIDFromPath(r.URL.Path, 3)
	var dep int
	if id == 0 || a.Store.DB.QueryRowContext(r.Context(), "SELECT department_id FROM generation_runs WHERE id=?", id).Scan(&dep) != nil || dep != u.DepartmentID {
		return deny(w)
	}
	return true
}
func idHasDepartment(ctx context.Context, db storage.Querier, kind string, id, dept int) bool {
	var owned int
	switch kind {
	case "programs", "faculty", "subjects", "buildings", "rooms":
		_ = db.QueryRowContext(ctx, "SELECT department_id FROM "+kind+" WHERE id=?", id).Scan(&owned)
	case "sections":
		_ = db.QueryRowContext(ctx, "SELECT p.department_id FROM sections s JOIN programs p ON p.id=s.program_id WHERE s.id=?", id).Scan(&owned)
	case "curricula":
		_ = db.QueryRowContext(ctx, "SELECT p.department_id FROM curricula c JOIN programs p ON p.id=c.program_id WHERE c.id=?", id).Scan(&owned)
	default:
		return false
	}
	return owned == dept
}
func ownsRow(ctx context.Context, db storage.Querier, dept int, kind string, row map[string]any) bool {
	id := domain.Int(row, "id")
	switch kind {
	case "programs", "faculty", "subjects", "buildings", "rooms":
		return domain.Int(row, "department_id") == dept
	case "sections", "curricula":
		return idHasDepartment(ctx, db, kind, id, dept)
	case "faculty_subjects", "faculty_considerations", "faculty_work_times", "faculty_preferences":
		return idHasDepartment(ctx, db, "faculty", domain.Int(row, "faculty_id"), dept)
	case "teaching_assignments":
		return idHasDepartment(ctx, db, "sections", domain.Int(row, "section_id"), dept)
	default:
		return false
	}
}

// Departmental moderators can discover shared GENED and cross-department
// teaching resources. Write ownership is checked separately above.
func scopeVisible(ctx context.Context, db storage.Querier, dept int, kind string, v map[string]any) bool {
	switch kind {
	case "departments", "academic_periods", "programs", "subjects", "subject_components", "subject_facility_requests", "faculty", "buildings", "rooms", "sections", "curricula", "curriculum_subject_years", "faculty_subjects", "faculty_work_times", "faculty_preferences", "faculty_considerations", "teaching_assignments", "scheduled_meetings":
		return true
	}
	return false
}
