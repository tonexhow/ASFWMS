package httpapi

import (
	"asfwms.local/scheduler/internal/security"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type roleSession struct {
	t     *testing.T
	h     *harness
	http  *http.Client
	token string
}

func newRoleSession(t *testing.T, h *harness, username, password string) *roleSession {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	v := &roleSession{t, h, client, ""}
	_, resp := v.call("POST", "/api/login", map[string]any{"username": username, "password": password}, 200)
	var u User
	if err := json.Unmarshal(resp, &u); err != nil {
		t.Fatal(err)
	}
	v.token = u.CSRF
	return v
}
func (s *roleSession) call(method, path string, body any, want int) (*http.Response, []byte) {
	s.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, s.h.server.URL+path, bytes.NewReader(raw))
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ASFWMS-Request", "1")
		req.Header.Set("X-CSRF-Token", s.token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s => %d wanted %d: %.200s", method, path, resp.StatusCode, want, b)
	}
	return resp, b
}
func TestAccessRolesPrimaryDeveloperProtectionAndRecoveryInbox(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, b := h.call("GET", "/api/access/users", nil, nil, 200)
	if !bytes.Contains(b, []byte("test_admin")) {
		t.Fatal("owner missing from own access list")
	}
	add := func(username, role string, dep int) {
		h.call("POST", "/api/access/users", map[string]any{"name": "Test " + username, "email": username + "@example.org", "username": username, "password": "Long-password-for-test-22", "role": role, "department_id": dep}, nil, 201)
	}
	add("dev_second", "developer", 0)
	add("admin_second", "administrator", 0)
	add("cite_scheduler", "scheduler", 1)
	second := newRoleSession(t, h, "dev_second", "Long-password-for-test-22")
	_, data := second.call("GET", "/api/access/users", nil, 200)
	var list struct {
		Users []map[string]any `json:"users"`
	}
	if e := json.Unmarshal(data, &list); e != nil {
		t.Fatal(e)
	}
	for _, u := range list.Users {
		if u["id"] == float64(1) && (u["email"] != "" || u["username"] != "(private)") {
			t.Fatal("protected developer identity exposed")
		}
	}
	second.call("PUT", "/api/access/users/1", map[string]any{"name": "Changed", "email": "evil@example.org", "username": "evil", "role": "scheduler", "department_id": 1}, 403)
	second.call("POST", "/api/access/users/1/disable", map[string]any{"confirm": "DISABLE"}, 403)
	second.call("POST", "/api/access/users/1/reset-email", map[string]any{"confirm": "SEND"}, 403)
	admin := newRoleSession(t, h, "admin_second", "Long-password-for-test-22")
	admin.call("GET", "/api/developer/database", nil, 403)
	admin.call("GET", "/api/developer/system", nil, 403)
	admin.call("POST", "/api/developer/clear-schedule", map[string]any{"department_id": 1, "period_id": 1, "confirm": "CLEAR SCHEDULE"}, 403)
	admin.call("POST", "/api/access/users", map[string]any{"name": "Forbidden", "email": "new@example.org", "username": "otherdev", "password": "Some password long enough", "role": "developer"}, 400)
	staff := newRoleSession(t, h, "cite_scheduler", "Long-password-for-test-22")
	staff.call("GET", "/api/access/users", nil, 403)
	staff.call("GET", "/api/developer/system", nil, 403)
	staff.call("GET", "/api/bootstrap", nil, 200)
	staff.call("POST", "/api/records/faculty", map[string]any{"name": "Invalid dept", "department_id": 2}, 403)
	staff.call("POST", "/api/records/faculty", map[string]any{"name": "Invalid unscoped"}, 403)
	staff.call("POST", "/api/faculty/2/section-policy", map[string]any{"policy": "exclusive"}, 403)
	staff.call("POST", "/api/subjects/1/faculty", map[string]any{"faculty_ids": []int{2}}, 409)
	staff.call("POST", "/api/generate", map[string]any{"department_id": 2, "period_id": 1, "mode": "standard", "seed": 42}, 403)
	staff.call("POST", "/api/account/reset-request", map[string]any{"message": "Lost password, request a reset link."}, 200)
	_, inbox := admin.call("GET", "/api/access/reset-requests", nil, 200)
	if !bytes.Contains(inbox, []byte("cite_scheduler")) {
		t.Fatal("access reset inbox did not record request")
	}
	admin.call("POST", "/api/access/users/4/reset-email", map[string]any{"confirm": "SEND"}, 503)
	h.call("GET", "/api/access/reset-requests", nil, nil, 200)
	_, b = h.call("GET", "/api/developer/database?table=users", nil, nil, 400)
	_ = b
}
func TestDeveloperDatabaseScheduleCleanupAndPrintPDF(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("GET", "/api/developer/database?table=faculty", nil, nil, 200)
	h.call("POST", "/api/developer/database", map[string]any{"table": "buildings", "operation": "update", "id": 1, "values": map[string]any{"name": "Revised building"}, "confirm": "APPLY"}, nil, 200)
	var buildingName string
	_ = h.store.DB.QueryRow("SELECT name FROM buildings WHERE id=1").Scan(&buildingName)
	if buildingName != "Revised building" {
		t.Fatal("developer edit not persisted")
	}
	id := h.generate(1)
	_, data := h.call("GET", "/api/timetable/pdf?"+url.Values{"view": {"sections"}, "period": {"1"}, "department": {"1"}, "run": {stringInt(id)}}.Encode(), nil, nil, 200)
	if target := os.Getenv("ASFWMS_PREVIEW_PDF"); target != "" {
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 5000 {
		t.Fatalf("PDF unavailable, %d bytes", len(data))
	}
	h.commit(id, 200)
	before := h.count("scheduled_meetings")
	if before == 0 {
		t.Fatal("nothing to clear")
	}
	h.call("POST", "/api/developer/clear-schedule", map[string]any{"department_id": 1, "period_id": 1, "confirm": "CLEAR SCHEDULE", "delete_history": false}, nil, 200)
	if h.count("scheduled_meetings") != 0 || h.count("teaching_assignments") != 2 {
		t.Fatal("cleanup erased faculty links or retained meetings")
	}
}
func stringInt(n int) string {
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	out := ""
	for n > 0 {
		out = string(digits[n%10]) + out
		n /= 10
	}
	return out
}
func TestAcademicPeriodDefaultsAndSectionPolicy(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/records/academic_periods", map[string]any{"academic_year": "2027-2028", "semester": "Summer", "term": ""}, nil, 200)
	var label, dates string
	var lunch, cap int
	if err := h.store.DB.QueryRow("SELECT label,start_date,lunch_minutes,max_consecutive FROM academic_periods WHERE academic_year='2027-2028'").Scan(&label, &dates, &lunch, &cap); err != nil {
		t.Fatal(err)
	}
	if label != "2027-2028 / Summer" || dates != "" || lunch != 60 || cap != 3 {
		t.Fatalf("wrong default period %s %s %d %d", label, dates, lunch, cap)
	}
	h.call("POST", "/api/faculty/1/section-policy", map[string]any{"policy": "exclusive"}, nil, 200)
	var policy string
	_ = h.store.DB.QueryRow("SELECT section_policy FROM faculty WHERE id=1").Scan(&policy)
	if policy != "exclusive" {
		t.Fatal("exclusive policy not persisted")
	}
	h.call("POST", "/api/faculty/1/section-policy", map[string]any{"policy": "priority"}, nil, 200)
}

// Recovery tokens are one-use, expiring, bound to unprotected accounts and revoke old sessions.
func TestPasswordRecoveryOneUseAndPrimaryAccountProtection(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/access/users", map[string]any{"name": "Other Person", "email": "reset@example.org", "username": "recoverme", "password": "Test-password-initial-2026", "role": "scheduler", "department_id": 1}, nil, 201)
	var id int
	if e := h.store.DB.QueryRow("SELECT id FROM users WHERE username='recoverme'").Scan(&id); e != nil {
		t.Fatal(e)
	}
	tok := strings.Repeat("a", 64)
	_, err := h.store.DB.Exec("INSERT INTO password_reset_tokens(token_hash,user_id,expires_at) VALUES(?,?,?)", security.Digest(tok), id, time.Now().Add(5*time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	h.call("POST", "/api/reset/confirm", map[string]any{"token": tok, "password": "Replacement-password-2026"}, nil, 200)
	h.call("POST", "/api/reset/confirm", map[string]any{"token": tok, "password": "Another-password-2026"}, nil, 409)
	staff := newRoleSession(t, h, "recoverme", "Replacement-password-2026")
	staff.call("GET", "/api/me", nil, 200)
	protected := strings.Repeat("b", 64)
	_, err = h.store.DB.Exec("INSERT INTO password_reset_tokens(token_hash,user_id,expires_at) VALUES(?,?,?)", security.Digest(protected), 1, time.Now().Add(5*time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	h.call("POST", "/api/reset/confirm", map[string]any{"token": protected, "password": "Attacker-new-password-2026"}, nil, 409)
	h.call("POST", "/api/login", map[string]any{"username": "test_admin", "password": "ASFWMS-test-password-42"}, nil, 200)
}
