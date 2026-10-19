package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/scheduling"
	"asfwms.local/scheduler/internal/timetable"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) schedulePDF(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("view")
	switch kind {
	case "section", "sections":
		kind = "sections"
	case "faculty":
	case "room", "rooms":
		kind = "rooms"
	default:
		problem(w, 400, "VIEW", "Choose section, faculty, or room schedule.")
		return
	}
	periodID, _ := strconv.Atoi(r.URL.Query().Get("period"))
	deptID, _ := strconv.Atoi(r.URL.Query().Get("department"))
	resourceID, _ := strconv.Atoi(r.URL.Query().Get("resource"))
	runID, _ := strconv.Atoi(r.URL.Query().Get("run"))
	if runID > 0 && user(r).Role == "scheduler" {
		var dep int
		if a.Store.DB.QueryRowContext(r.Context(), "SELECT department_id FROM generation_runs WHERE id=?", runID).Scan(&dep) != nil || dep != user(r).DepartmentID {
			deny(w)
			return
		}
	}
	if user(r).Role == "scheduler" && deptID > 0 && deptID != user(r).DepartmentID {
		deny(w)
		return
	}
	snap, _, err := domain.Load(r.Context(), a.Store.DB)
	if err != nil {
		problem(w, 500, "PDF", "Cannot load schedule source.")
		return
	}
	meetings := []domain.Meeting{}
	label := "Department schedules"
	committed := runID == 0
	period := domain.Period{}
	if runID > 0 {
		var s, status string
		if err = a.Store.DB.QueryRowContext(r.Context(), "SELECT result_json,status FROM generation_runs WHERE id=?", runID).Scan(&s, &status); err != nil {
			problem(w, 404, "RUN", "Schedule run not found.")
			return
		}
		var result scheduling.Result
		if err = json.Unmarshal([]byte(s), &result); err != nil {
			problem(w, 500, "RUN", "Cannot read stored schedule.")
			return
		}
		committed = status == "committed"
		meetings = result.Meetings
		periodID = result.Request.PeriodID
		deptID = result.Request.DepartmentID
		label = fmt.Sprintf("Generation #%d / %s", runID, result.Outcome)
	} else {
		for _, m := range snap.Meetings {
			if (periodID == 0 || m.PeriodID == periodID) && (deptID == 0 || m.DepartmentID == deptID) {
				meetings = append(meetings, m)
			}
		}
	}
	for _, p := range snap.Periods {
		if p.ID == periodID {
			period = p
			break
		}
	}
	if periodID == 0 {
		problem(w, 400, "PERIOD", "Choose an academic period.")
		return
	}
	if user(r).Role == "scheduler" {
		permitted := []domain.Meeting{}
		for _, m := range meetings {
			if m.DepartmentID == user(r).DepartmentID {
				permitted = append(permitted, m)
			}
		}
		meetings = permitted
	}
	selected := []domain.Meeting{}
	for _, m := range meetings {
		id := m.SectionID
		if kind == "faculty" {
			id = m.FacultyID
		}
		if kind == "rooms" {
			id = m.RoomID
		}
		if resourceID == 0 || id == resourceID {
			selected = append(selected, m)
		}
	}
	if resourceID != 0 {
		for _, m := range selected {
			_ = m
		}
		label = kind + " / " + strconv.Itoa(resourceID)
	}
	entries := timetableEntries(snap, selected)
	frame := timetable.Frame{Term: period.Label, AcademicYear: period.AcademicYear, Semester: period.Semester, PrintedAt: time.Now().Format("2006-01-02"), Version: runID}
	if !committed {
		frame.Version = 0
	} else if frame.Version == 0 {
		frame.Version = 1
	}
	template := r.URL.Query().Get("template")
	if template == "" {
		template = "split-classic"
	}
	if _, ok := timetable.ResolveTemplate(template); !ok {
		problem(w, 400, "TEMPLATE", "Unknown printable layout.")
		return
	}
	if err = timetable.LoadBranding(filepath.Join(a.Config.Root, "client", "assets")); err != nil {
		problem(w, 500, "BRANDING", "PDF branding assets unavailable.")
		return
	}
	result, err := timetable.PDFWithTemplate(kind, label, frame, entries, template)
	if err != nil {
		problem(w, 500, "PDF", "PDF could not render this text: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=ASFWMS_%s_%d.pdf", disposition, strings.ToLower(kind), time.Now().Unix()))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(200)
	_, _ = w.Write(result)
}
func getDept(s domain.Snapshot, id int) domain.Department {
	for _, x := range s.Departments {
		if x.ID == id {
			return x
		}
	}
	return domain.Department{}
}
func getProgram(s domain.Snapshot, id int) domain.Program {
	for _, x := range s.Programs {
		if x.ID == id {
			return x
		}
	}
	return domain.Program{}
}
func getSection(s domain.Snapshot, id int) domain.Section {
	for _, x := range s.Sections {
		if x.ID == id {
			return x
		}
	}
	return domain.Section{}
}
func getFaculty(s domain.Snapshot, id int) domain.Faculty {
	for _, x := range s.Faculty {
		if x.ID == id {
			return x
		}
	}
	return domain.Faculty{}
}
func getRoom(s domain.Snapshot, id int) domain.Room {
	for _, x := range s.Rooms {
		if x.ID == id {
			return x
		}
	}
	return domain.Room{}
}
func getSubject(s domain.Snapshot, id int) domain.Subject {
	for _, x := range s.Subjects {
		if x.ID == id {
			return x
		}
	}
	return domain.Subject{}
}
