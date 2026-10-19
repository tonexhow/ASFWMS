package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"asfwms.local/scheduler/internal/timetable"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// emailFacultyTimetable sends only a committed, current faculty schedule to
// the address stored in that faculty record. The caller cannot substitute a
// recipient address. This avoids accidentally sending private schedules to
// an arbitrary contact or an unverified address typed into a preview.
func (a *App) emailFacultyTimetable(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FacultyID    int    `json:"faculty_id"`
		PeriodID     int    `json:"period_id"`
		DepartmentID int    `json:"department_id"`
		Template     string `json:"template"`
		Confirm      string `json:"confirm"`
	}
	if decode(r, &in) != nil || in.FacultyID < 1 || in.PeriodID < 1 || in.DepartmentID < 0 || in.Confirm != "SEND" {
		problem(w, 400, "INPUT", "Select a faculty schedule and explicitly confirm SEND.")
		return
	}
	if _, ok := timetable.ResolveTemplate(in.Template); !ok {
		problem(w, 400, "TEMPLATE", "Choose a supported timetable layout.")
		return
	}
	if a.limited(r) {
		problem(w, 429, "RATE_LIMIT", "Too many email requests. Wait one minute before trying again.")
		return
	}
	if user(r).Role == "scheduler" {
		if in.DepartmentID != 0 && in.DepartmentID != user(r).DepartmentID {
			deny(w)
			return
		}
		in.DepartmentID = user(r).DepartmentID
	}
	s, _, err := domain.Load(r.Context(), a.Store.DB)
	if err != nil {
		problem(w, 503, "DATABASE", "Could not read the committed timetable.")
		return
	}
	faculty := getFaculty(s, in.FacultyID)
	if faculty.ID == 0 {
		problem(w, 404, "FACULTY", "Faculty record not found.")
		return
	}
	if user(r).Role == "scheduler" && faculty.DepartmentID != user(r).DepartmentID {
		deny(w)
		return
	}
	recipient := strings.TrimSpace(faculty.Email)
	if !validMailbox(recipient) {
		problem(w, 409, "FACULTY_EMAIL", "Add a valid email to this faculty record before sending a timetable.")
		return
	}
	var period domain.Period
	for _, p := range s.Periods {
		if p.ID == in.PeriodID {
			period = p
			break
		}
	}
	if period.ID == 0 {
		problem(w, 404, "PERIOD", "Academic period not found.")
		return
	}
	selected := selectCommittedFacultyMeetings(s.Meetings, in.FacultyID, in.PeriodID, in.DepartmentID)
	if len(selected) == 0 {
		problem(w, 404, "EMPTY", "No committed meetings are available for this faculty and academic period.")
		return
	}
	if a.Config.SMTPEmail == "" || a.Config.SMTPPass == "" || !validMailbox(a.Config.SMTPEmail) {
		problem(w, 503, "SMTP_CONFIG", "SMTP is not configured. Add SMTP_EMAIL and SMTP_PASS in .env after hosting.")
		return
	}
	if err = timetable.LoadBranding(filepath.Join(a.Config.Root, "client", "assets")); err != nil {
		problem(w, 500, "PDF", "School branding assets are unavailable.")
		return
	}
	frame := timetable.Frame{Term: period.Label, AcademicYear: period.AcademicYear, Semester: period.Semester, PrintedAt: time.Now().Format("2006-01-02"), Version: 1}
	pdf, err := timetable.PDFWithTemplate("faculty", faculty.Name, frame, timetableEntries(s, selected), in.Template)
	if err != nil {
		problem(w, 500, "PDF", "Could not render the faculty timetable. No email was sent.")
		return
	}
	if len(pdf) > 12<<20 {
		problem(w, 413, "ATTACHMENT", "The PDF exceeds the supported email size.")
		return
	}
	title := fmt.Sprintf("BCC Faculty Schedule / %s / %s", faculty.Name, period.Label)
	body := fmt.Sprintf("Attached is your official ASFWMS faculty schedule.\r\nFaculty: %s\r\nAcademic period: %s\r\nWeekly meetings: %d\r\nThis PDF reflects the committed schedule at the time of sending.\r\n", faculty.Name, period.Label, len(selected))
	message, err := scheduleMessage(a.Config.SMTPEmail, recipient, title, body, fmt.Sprintf("BCC_Faculty_%d_Period_%d.pdf", faculty.ID, period.ID), pdf)
	if err != nil {
		problem(w, 500, "EMAIL", "Could not prepare the email. Nothing was sent.")
		return
	}
	host, port := a.Config.SMTPHost, a.Config.SMTPPort
	if host == "" {
		host = "smtp.gmail.com"
	}
	if port == "" {
		port = "587"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err = sendScheduleSMTP(ctx, host, port, a.Config.SMTPEmail, a.Config.SMTPPass, recipient, message); err != nil {
		problem(w, 502, "SMTP_SEND", "SMTP delivery could not be confirmed. Check provider logs before retrying to avoid duplicates.")
		return
	}
	logged := a.Store.Write(r.Context(), func(q storage.Querier) error {
		return audit(r.Context(), q, user(r), "email-faculty-timetable", "faculty", faculty.ID, fmt.Sprintf("SMTP accepted; faculty=%d; period=%d; department=%d; recipient=%s", faculty.ID, period.ID, in.DepartmentID, recipient))
	}) == nil
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "smtp_accepted": true, "audit_saved": logged, "message": "SMTP accepted the faculty schedule. Inbox delivery is not guaranteed."})
}

func selectCommittedFacultyMeetings(all []domain.Meeting, facultyID, periodID, departmentID int) []domain.Meeting {
	selected := make([]domain.Meeting, 0)
	for _, m := range all {
		if m.FacultyID == facultyID && m.PeriodID == periodID && (departmentID == 0 || m.DepartmentID == departmentID) {
			selected = append(selected, m)
		}
	}
	return selected
}
