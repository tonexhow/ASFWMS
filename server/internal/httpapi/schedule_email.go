package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"asfwms.local/scheduler/internal/timetable"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

func timetableEntries(s domain.Snapshot, meetings []domain.Meeting) []timetable.Entry {
	entries := []timetable.Entry{}
	components := map[int]domain.SubjectComponent{}
	for _, c := range s.Components {
		components[c.ID] = c
	}
	for _, m := range meetings {
		section, subject, faculty, room := getSection(s, m.SectionID), getSubject(s, m.SubjectID), getFaculty(s, m.FacultyID), getRoom(s, m.RoomID)
		kind, units := room.Kind, subject.Units
		if c, ok := components[m.ComponentID]; ok {
			kind = c.Kind
			units = c.Units
		} else if m.ComponentKind != "" {
			kind = m.ComponentKind
		}
		entries = append(entries, timetable.Entry{SectionID: int64(m.SectionID), OfferingID: int64(m.SectionID)*1000000 + int64(m.SubjectID), FacultyID: int64(m.FacultyID), RoomID: int64(m.RoomID), Faculty: faculty.Name, Room: room.Code, Department: getDept(s, m.DepartmentID).Code, Program: getProgram(s, section.ProgramID).Code, Section: section.Code, SubjectCode: subject.Code, SubjectTitle: subject.Title, Kind: kind, YearLevel: section.Year, Day: m.Day, Start: m.Start, End: m.End, Units: units})
	}
	return entries
}
func validMailbox(address string) bool {
	if strings.ContainsAny(address, "\r\n") || len(address) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == address
}

// scheduleEmail only accepts a committed, authorized run. It never commits a
// preview and never guesses a recipient or sends to all faculty automatically.
func (a *App) scheduleEmail(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Recipient  string `json:"recipient"`
		View       string `json:"view"`
		ResourceID int    `json:"resource_id"`
		Confirm    string `json:"confirm"`
	}
	if decode(r, &in) != nil || in.Confirm != "SEND" || !validMailbox(strings.TrimSpace(in.Recipient)) {
		problem(w, 400, "RECIPIENT", "Enter one valid email address and explicitly confirm SEND.")
		return
	}
	in.Recipient = strings.TrimSpace(in.Recipient)
	if in.View != "sections" && in.View != "rooms" && in.View != "faculty" || in.ResourceID < 0 {
		problem(w, 400, "VIEW", "Choose section, room or faculty schedules.")
		return
	}
	result, status, err := a.readGeneration(r.Context(), id)
	if err != nil {
		problem(w, 404, "RUN", err.Error())
		return
	}
	if status != "committed" {
		problem(w, 409, "COMMIT", "Commit this schedule before sending it by email.")
		return
	}
	// The route is also scoped by authorizeRoute. Recheck here for direct reuse.
	if user(r).Role == "scheduler" && result.Request.DepartmentID != user(r).DepartmentID {
		deny(w)
		return
	}
	if a.Config.SMTPEmail == "" || a.Config.SMTPPass == "" || !validMailbox(a.Config.SMTPEmail) {
		problem(w, 503, "SMTP_CONFIG", "Email is not configured. Set SMTP_EMAIL and SMTP_PASS in your private .env, then restart ASFWMS.")
		return
	}
	s, _, err := domain.Load(r.Context(), a.Store.DB)
	if err != nil {
		problem(w, 503, "DATABASE", "Could not read the committed timetable.")
		return
	}
	selected := []domain.Meeting{}
	for _, m := range result.Meetings {
		resource := m.SectionID
		if in.View == "rooms" {
			resource = m.RoomID
		}
		if in.View == "faculty" {
			resource = m.FacultyID
		}
		if in.ResourceID == 0 || resource == in.ResourceID {
			selected = append(selected, m)
		}
	}
	if len(selected) == 0 {
		problem(w, 404, "EMPTY", "No meetings match this run and resource selection.")
		return
	}
	var period domain.Period
	for _, p := range s.Periods {
		if p.ID == result.Request.PeriodID {
			period = p
			break
		}
	}
	if err = timetable.LoadBranding(filepath.Join(a.Config.Root, "client", "assets")); err != nil {
		problem(w, 500, "PDF", "The school-header assets are unavailable.")
		return
	}
	title := fmt.Sprintf("ASFWMS / %s / %s / Run %d", getDept(s, result.Request.DepartmentID).Code, period.Label, id)
	frame := timetable.Frame{Term: period.Label, AcademicYear: period.AcademicYear, Semester: period.Semester, PrintedAt: time.Now().Format("2006-01-02"), Version: id}
	pdf, err := timetable.PDFWithTemplate(in.View, title, frame, timetableEntries(s, selected), "split-classic")
	if err != nil {
		problem(w, 500, "PDF", "The timetable could not be rendered. No email was sent.")
		return
	}
	if len(pdf) > 12<<20 {
		problem(w, 413, "ATTACHMENT", "The PDF is too large for email. Select one section, faculty or room.")
		return
	}
	filename := fmt.Sprintf("ASFWMS_%s_run_%d.pdf", in.View, id)
	body := fmt.Sprintf("Attached is the committed ASFWMS timetable for %s.\r\nAcademic period: %s\r\nRun: %d\r\nView: %s\r\nMeetings: %d\r\nThis attachment is a saved schedule, not a live reservation update.\r\n", getDept(s, result.Request.DepartmentID).Name, period.Label, id, in.View, len(selected))
	message, err := scheduleMessage(a.Config.SMTPEmail, in.Recipient, title, body, filename, pdf)
	if err != nil {
		problem(w, 500, "EMAIL", "Could not prepare the email. Nothing was sent.")
		return
	}
	host := a.Config.SMTPHost
	if host == "" {
		host = "smtp.gmail.com"
	}
	port := a.Config.SMTPPort
	if port == "" {
		port = "587"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err = sendScheduleSMTP(ctx, host, port, a.Config.SMTPEmail, a.Config.SMTPPass, in.Recipient, message); err != nil {
		// Never echo authentication errors or server strings containing account
		// data. The provider may have accepted a DATA command before a timeout.
		problem(w, 502, "SMTP_SEND", "SMTP delivery could not be confirmed. Check the sender's sent mail/provider log before retrying to avoid duplicates. Check SMTP credentials and outbound SMTP access.")
		return
	}
	auditErr := a.Store.Write(r.Context(), func(q storage.Querier) error {
		return audit(r.Context(), q, user(r), "email-timetable", "generation_runs", id, fmt.Sprintf("SMTP accepted; recipient=%s; view=%s; resource=%d", in.Recipient, in.View, in.ResourceID))
	})
	writeJSON(w, 200, map[string]any{"ok": true, "smtp_accepted": true, "audit_saved": auditErr == nil, "message": "SMTP server accepted the message. Inbox delivery is not guaranteed."})
}
func scheduleMessage(from, to, subject, text, filename string, pdf []byte) ([]byte, error) {
	if !validMailbox(from) || !validMailbox(to) || strings.ContainsAny(subject+filename, "\r\n") {
		return nil, fmt.Errorf("invalid email headers")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", "text/plain; charset=UTF-8")
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(wrappedBase64([]byte(text))); err != nil {
		return nil, err
	}
	header = textproto.MIMEHeader{}
	header.Set("Content-Type", "application/pdf")
	header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	header.Set("Content-Transfer-Encoding", "base64")
	part, err = writer.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(wrappedBase64(pdf)); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n", from, to, mime.QEncoding.Encode("UTF-8", subject), time.Now().Format(time.RFC1123Z), writer.Boundary())
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
func wrappedBase64(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	var out bytes.Buffer
	for len(encoded) > 0 {
		n := 76
		if n > len(encoded) {
			n = len(encoded)
		}
		out.WriteString(encoded[:n])
		out.WriteString("\r\n")
		encoded = encoded[n:]
	}
	return out.Bytes()
}
func sendScheduleSMTP(ctx context.Context, host, port, username, password, to string, message []byte) error {
	connection, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline := time.Now().Add(25 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err = connection.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
		return err
	}
	if err = client.Mail(username); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	data, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = data.Write(message); err != nil {
		return err
	}
	if err = data.Close(); err != nil {
		return err
	}
	// A successful DATA close is acceptance, even when QUIT subsequently fails.
	_ = client.Quit()
	return nil
}
