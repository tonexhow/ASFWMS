package httpapi

import (
	"asfwms.local/scheduler/internal/scheduling"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestSubjectComponentsAtomicPersistenceAndCommit(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, err := h.store.DB.Exec(`INSERT INTO rooms(id,building_id,department_id,code,name,kind,capacity) VALUES(3,1,1,'LAB','Computer Lab','laboratory',50)`)
	if err != nil {
		t.Fatal(err)
	}
	plan := map[string]any{"components": []map[string]any{{"kind": "lecture", "units": 1}, {"kind": "laboratory", "units": 2}}, "requested_facility_ids": []int{3}}
	h.call("PUT", "/api/records/subjects/1", plan, nil, 200)
	var total, minutes int
	h.store.DB.QueryRow("SELECT units,weekly_minutes FROM subjects WHERE id=1").Scan(&total, &minutes)
	if total != 3 || minutes != 180 || h.count("subject_components") != 2 {
		t.Fatal("component totals not saved")
	}
	h.call("PUT", "/api/records/subjects/1", map[string]any{"weekly_minutes": 240, "units": 4}, nil, 409)
	// Removing a type without clearing its facility request must roll back atomically.
	h.call("PUT", "/api/records/subjects/1", map[string]any{"components": []map[string]any{{"kind": "lecture", "units": 1}}}, nil, 409)
	h.store.DB.QueryRow("SELECT weekly_minutes FROM subjects WHERE id=1").Scan(&minutes)
	if minutes != 180 || h.count("subject_components") != 2 {
		t.Fatal("failed update partially persisted")
	}
	id := h.generate(1)
	h.commit(id, 200)
	var valid int
	h.store.DB.QueryRow("SELECT count(*) FROM scheduled_meetings WHERE component_id IS NOT NULL AND component_kind IN('lecture','laboratory')").Scan(&valid)
	if valid == 0 || valid != h.count("scheduled_meetings") {
		t.Fatal("committed components missing")
	}
	h.call("PUT", "/api/records/subjects/1", map[string]any{"components": []map[string]any{{"kind": "lecture", "units": 2}, {"kind": "laboratory", "units": 2}}, "requested_facility_ids": []int{3}}, nil, 409)
}
func TestFacultyStructuredNamesOptionalEmailAndLegacyPreservation(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, body := h.call("POST", "/api/records/faculty", map[string]any{"prefix": "Dr.", "first_name": "Ana Maria", "middle_name": "L.", "last_name": "Dela Cruz", "extension": "Jr.", "email": "ana@example.edu", "department_id": 1, "employment": "Full Time"}, nil, 200)
	if !bytes.Contains(body, []byte("Dr. Ana Maria L. Dela Cruz Jr.")) {
		t.Fatal(string(body))
	}
	h.call("POST", "/api/records/faculty", map[string]any{"first_name": "Ana", "last_name": ""}, nil, 409)
	h.call("PUT", "/api/records/faculty/1", map[string]any{"email": "bad address"}, nil, 409)
	h.call("PUT", "/api/records/faculty/1", map[string]any{"email": "", "employment": "Part Time"}, nil, 200)
	var name, first string
	h.store.DB.QueryRow("SELECT name,first_name FROM faculty WHERE id=1").Scan(&name, &first)
	if name != "Instructor One" || first != "" {
		t.Fatal("legacy name altered")
	}
	h.call("POST", "/api/faculty/1/preferences", map[string]any{"preferred_room_id": 999}, nil, 409)
}
func TestFacultySectionsSavePolicyAndAllChoicesAtomically(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	// A valid early selection followed by an invalid one may not alter policy or assignments.
	h.call("POST", "/api/faculty/1/sections", map[string]any{"policy": "exclusive", "subjects": []map[string]any{{"subject_id": 1, "section_ids": []int{1, 2}}}}, nil, 409)
	var policy string
	h.store.DB.QueryRow("SELECT section_policy FROM faculty WHERE id=1").Scan(&policy)
	if policy != "priority" || h.count("teaching_assignments") != 2 {
		t.Fatal("partial section save")
	}
	h.call("POST", "/api/faculty/1/sections", map[string]any{"policy": "exclusive", "subjects": []map[string]any{{"subject_id": 1, "section_ids": []int{1}}}}, nil, 200)
	h.store.DB.QueryRow("SELECT section_policy FROM faculty WHERE id=1").Scan(&policy)
	if policy != "exclusive" {
		t.Fatal(policy)
	}
	h.call("POST", "/api/faculty/1/sections", map[string]any{"policy": "exclusive", "subjects": []any{}}, nil, 409)
	h.call("POST", "/api/faculty/1/sections", map[string]any{"policy": "priority", "subjects": []any{}}, nil, 200)
	if h.count("teaching_assignments") != 1 {
		t.Fatal("outer save did not apply removals")
	}
}
func TestModeratorSharedDirectoriesAndWriteOwnership(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, err := h.store.DB.Exec(`INSERT INTO departments(id,code,name) VALUES(3,'GENED','General Education'); INSERT INTO subjects(id,department_id,code,title) VALUES(3,3,'GE1','Shared course'); INSERT INTO faculty(id,department_id,name) VALUES(4,3,'Shared Instructor');`)
	if err != nil {
		t.Fatal(err)
	}
	h.call("POST", "/api/access/users", map[string]any{"name": "CITE Moderator", "email": "moderator@example.edu", "username": "mod_test", "password": "Strong-test-password-123", "role": "scheduler", "department_id": 1}, nil, 201)
	mod := newRoleSession(t, h, "mod_test", "Strong-test-password-123")
	_, body := mod.call("GET", "/api/bootstrap", nil, 200)
	for _, word := range []string{"Instructor Two", "Shared Instructor", "Shared course", "Teaching"} {
		if !bytes.Contains(body, []byte(word)) {
			t.Fatal("directory hidden", word)
		}
	}
	mod.call("PUT", "/api/records/faculty/2", map[string]any{"name": "Unauthorized"}, 403)
	mod.call("PUT", "/api/records/subjects/1", map[string]any{"department_id": nil}, 403)
	mod.call("POST", "/api/faculty/1/subjects", map[string]any{"subject_ids": []int{1, 3}}, 200)
	// Linking an owned instructor to another department's subject is permitted;
	// the foreign faculty membership is retained, not silently removed.
	mod.call("POST", "/api/subjects/2/faculty", map[string]any{"faculty_ids": []int{1, 2}}, 200)
	var links int
	h.store.DB.QueryRow("SELECT count(*) FROM faculty_subjects WHERE subject_id=2").Scan(&links)
	if links != 2 {
		t.Fatal("cross-department eligibility not preserved")
	}
}
func TestLiveStreamProgressAndForceConfirmation(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/generate", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "force"}, nil, 400)
	response, body := h.call("POST", "/api/generate/stream", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "standard", Seed: 42}, nil, 200)
	if !strings.Contains(response.Header.Get("Content-Type"), "ndjson") {
		t.Fatal("not streamed")
	}
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	last := -1
	progress, result := 0, 0
	for _, line := range lines {
		var event struct {
			Type      string `json:"type"`
			Processed int    `json:"processed"`
			Total     int    `json:"total"`
			Run       struct {
				ID int `json:"id"`
			} `json:"run"`
		}
		if json.Unmarshal(line, &event) != nil {
			t.Fatal(string(line))
		}
		switch event.Type {
		case "progress":
			progress++
			if event.Processed < last || event.Processed > event.Total {
				t.Fatal("bad counts")
			}
			last = event.Processed
		case "result":
			result++
			if event.Run.ID <= 0 {
				t.Fatal("missing persisted preview")
			}
		default:
			t.Fatal(string(line))
		}
	}
	if progress < 3 || result != 1 || h.count("scheduled_meetings") != 0 {
		t.Fatal("fake events or premature commit")
	}
}
func TestWaitResumesToPreviewOnlyAfterOtherDepartmentCommit(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	if _, err := h.store.DB.Exec("UPDATE rooms SET department_id=2 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	_, body := h.call("POST", "/api/generate", scheduling.Request{DepartmentID: 1, PeriodID: 1, Mode: "standard", Seed: 42}, nil, 201)
	var run struct {
		ID     int               `json:"id"`
		Result scheduling.Result `json:"result"`
	}
	json.Unmarshal(body, &run)
	if run.Result.Placed != 0 || len(run.Result.Issues) == 0 || run.Result.Issues[0].Code != "LACK_OF_FACILITY" {
		t.Fatal(string(body))
	}
	h.call("POST", fmt.Sprintf("/api/runs/%d/wait", run.ID), map[string]any{"confirm": "WAIT"}, nil, 200)
	h.call("GET", "/api/waits", nil, nil, 200)
	var status string
	h.store.DB.QueryRow("SELECT status FROM generation_waits").Scan(&status)
	if status != "pending" {
		t.Fatal("Wait proceeded before completion", status)
	}
	other := h.generate(2)
	h.commit(other, 200)
	latest := 0
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.call("GET", "/api/waits", nil, nil, 200)
		h.store.DB.QueryRow("SELECT status,COALESCE(latest_run_id,0) FROM generation_waits").Scan(&status, &latest)
		if status == "ready" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != "ready" || latest == 0 {
		t.Fatalf("wait did not resume: %s %d", status, latest)
	}
	var own int
	h.store.DB.QueryRow("SELECT count(*) FROM scheduled_meetings WHERE department_id=1").Scan(&own)
	if own != 0 {
		t.Fatal("queue auto-committed")
	}
	_, body = h.call("GET", fmt.Sprintf("/api/runs/%d", latest), nil, nil, 200)
	json.Unmarshal(body, &run)
	if run.Result.Placed != 1 || run.Result.Borrowed == 0 {
		t.Fatal("borrowing preview not filled", string(body))
	}
	h.commit(latest, 200)
	h.store.DB.QueryRow("SELECT status FROM generation_waits").Scan(&status)
	if status != "resolved" {
		t.Fatal(status)
	}
}
func TestScheduleEmailRequiresCommitConfigurationAndExplicitRecipient(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	id := h.generate(1)
	path := fmt.Sprintf("/api/runs/%d/email", id)
	payload := map[string]any{"recipient": "receiver@example.edu", "view": "sections", "confirm": "SEND"}
	h.call("POST", path, payload, nil, 409)
	h.commit(id, 200)
	h.call("POST", path, payload, nil, 503)
	payload["recipient"] = "recipient@example.edu\r\nBcc:other@example.edu"
	h.call("POST", path, payload, nil, 400)
	payload["recipient"] = "receiver@example.edu"
	payload["confirm"] = ""
	h.call("POST", path, payload, nil, 400)
}
func TestScheduleMessageHasExactPDFAndRejectsHeaderInjection(t *testing.T) {
	pdf := []byte("%PDF-1.4\nTest fixture\n%%EOF")
	raw, err := scheduleMessage("sender@example.edu", "receiver@example.edu", "ASFWMS committed timetable", "Schedule attached.", "schedule.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	count := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		if err != nil {
			t.Fatal(err)
		}
		if part.Header.Get("Content-Type") == "application/pdf" {
			count++
			if !bytes.Equal(data, pdf) {
				t.Fatal("PDF attachment corrupted")
			}
		}
	}
	if count != 1 {
		t.Fatal("missing or duplicate PDF")
	}
	if _, err = scheduleMessage("sender@example.edu", "receiver@example.edu", "Hello\r\nBcc:other@example.edu", "", "file.pdf", pdf); err == nil {
		t.Fatal("header injection accepted")
	}
}
