package httpapi

import (
	"asfwms.local/scheduler/internal/timetable"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestFacultyEmailRequiresSavedEmailCommittedScheduleAndConfirmation(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "template": "split-classic", "confirm": "SEND"}, nil, 409)
	h.store.DB.Exec("UPDATE faculty SET email=? WHERE id=1", "instructor@example.edu")
	h.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "template": "split-classic", "confirm": "SEND"}, nil, 404)
	id := h.generate(1)
	h.commit(id, 200)
	for _, body := range []map[string]any{
		{"faculty_id": 1, "period_id": 1, "template": "split-classic", "confirm": ""},
		{"faculty_id": 1, "period_id": 1, "template": "broken-theme", "confirm": "SEND"},
	} {
		h.call("POST", "/api/timetable/faculty/email", body, nil, 400)
	}
	h.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "template": "split-classic", "recipient": "attacker@example.org", "confirm": "SEND"}, nil, 400)
	h.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "template": "split-classic", "confirm": "SEND"}, nil, 503)
	h.call("GET", "/api/timetable/faculty/email", nil, nil, 404)
}

func TestFacultyEmailModeratorCannotSendAnotherDepartment(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.store.DB.Exec("UPDATE faculty SET email=? WHERE id IN (1,2)", "instructor@example.edu")
	id := h.generate(1)
	h.commit(id, 200)
	h.call("POST", "/api/access/users", map[string]any{"name": "CITE Moderator", "email": "mod_email@example.edu", "username": "mod_email", "password": "Strong-test-password-123", "role": "scheduler", "department_id": 1}, nil, 201)
	mod := newRoleSession(t, h, "mod_email", "Strong-test-password-123")
	mod.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 2, "period_id": 1, "template": "split-classic", "confirm": "SEND"}, 403)
	mod.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "department_id": 2, "template": "split-classic", "confirm": "SEND"}, 403)
	mod.call("POST", "/api/timetable/faculty/email", map[string]any{"faculty_id": 1, "period_id": 1, "template": "split-classic", "confirm": "SEND"}, 503)
}

func TestOfficialTimetableTemplatesPDFAndInlinePrint(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	id := h.generate(1)
	h.commit(id, 200)
	for _, spec := range timetable.TemplateList() {
		path := fmt.Sprintf("/api/timetable/pdf?view=faculty&period=1&resource=1&template=%s&inline=1", spec.ID)
		response, data := h.call("GET", path, nil, nil, 200)
		if !strings.HasPrefix(response.Header.Get("Content-Disposition"), "inline;") {
			t.Fatalf("%s print not inline", spec.ID)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatalf("%s invalid PDF", spec.ID)
		}
		if !bytes.Contains(data, []byte("registrarofbcc@yahoo.com")) {
			t.Fatalf("%s missing BCC header contact", spec.ID)
		}
	}
	res, _ := h.call("GET", "/api/timetable/pdf?view=faculty&period=1&resource=1&template=split-maroon", nil, nil, 200)
	if !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment;") {
		t.Fatal("download should force attachment")
	}
}
