package httpapi

import (
	"asfwms.local/scheduler/internal/config"
	"asfwms.local/scheduler/internal/scheduling"
	"asfwms.local/scheduler/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type harness struct {
	t      *testing.T
	store  *storage.Store
	server *httptest.Server
	client *http.Client
	csrf   string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	s, err := storage.Open(filepath.Join(t.TempDir(), "database.db"), filepath.Join(root, "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, store: s}
	h.server = httptest.NewServer(New(s, config.Config{Root: root, Host: "127.0.0.1", Port: "20000"}).Handler())
	jar, _ := cookiejar.New(nil)
	h.client = &http.Client{Jar: jar}
	t.Cleanup(func() { h.server.Close(); s.Close() })
	_, body := h.call("POST", "/api/setup", map[string]any{"username": "test_admin", "password": "ASFWMS-test-password-42"}, nil, 201)
	var u User
	if err = json.Unmarshal(body, &u); err != nil {
		t.Fatal(err)
	}
	h.csrf = u.CSRF
	return h
}
func (h *harness) call(method, path string, body any, headers map[string]string, want int) (*http.Response, []byte) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, h.server.URL+path, bytes.NewReader(raw))
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ASFWMS-Request", "1")
		req.Header.Set("X-CSRF-Token", h.csrf)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	res, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	if res.StatusCode != want {
		h.t.Fatalf("%s %s: got %d want %d: %s", method, path, res.StatusCode, want, data)
	}
	return res, data
}
func (h *harness) fixture() {
	h.t.Helper()
	_, err := h.store.DB.Exec(`
 INSERT INTO departments(id,code,name) VALUES(1,'CITE','Technology'),(2,'COTE','Education');
 INSERT INTO programs(id,department_id,code,name) VALUES(1,1,'BSIT','IT'),(2,2,'BSED','Education');
 INSERT INTO academic_periods(id,label,academic_year,semester,start_date,end_date) VALUES(1,'Test period','2026-2027','First','2026-08-01','2026-12-20');
 INSERT INTO subjects(id,department_id,code,title,weekly_minutes,pattern) VALUES(1,1,'IT101','Programming',180,'mwf'),(2,2,'ED101','Teaching',180,'mwf');
 INSERT INTO curricula(id,program_id,name) VALUES(1,1,'IT curriculum'),(2,2,'Education curriculum');
 INSERT INTO curriculum_subject_years(id,curriculum_id,subject_id,year_level) VALUES(1,1,1,1),(2,2,2,1);
 INSERT INTO faculty(id,department_id,name) VALUES(1,1,'Instructor One'),(2,2,'Instructor Two'),(3,1,'Independent Adviser');
 INSERT INTO sections(id,program_id,curriculum_id,period_id,code,year_level,size,adviser_id) VALUES(1,1,1,1,'IT-A',1,30,3),(2,2,2,1,'ED-A',1,30,NULL);
 INSERT INTO faculty_subjects(id,faculty_id,subject_id) VALUES(1,1,1),(2,2,2),(3,2,1);
 INSERT INTO teaching_assignments(id,faculty_id,subject_id,section_id) VALUES(1,1,1,1),(2,2,2,2);
 INSERT INTO buildings(id,code,name) VALUES(1,'B','Building');
 INSERT INTO rooms(id,building_id,department_id,code,name,capacity) VALUES(1,1,1,'R1','Room One',40),(2,1,2,'R2','Room Two',40);
 `)
	if err != nil {
		h.t.Fatal(err)
	}
}
func (h *harness) generate(dept int) int {
	h.t.Helper()
	_, body := h.call("POST", "/api/generate", scheduling.Request{DepartmentID: dept, PeriodID: 1, Mode: "standard", Seed: 42}, nil, 201)
	var v struct {
		ID     int               `json:"id"`
		Result scheduling.Result `json:"result"`
	}
	if e := json.Unmarshal(body, &v); e != nil {
		h.t.Fatal(e)
	}
	if v.Result.Placed == 0 {
		h.t.Fatal(string(body))
	}
	return v.ID
}
func (h *harness) count(table string) int {
	h.t.Helper()
	var n int
	if e := h.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil {
		h.t.Fatal(e)
	}
	return n
}
func (h *harness) commit(id int, want int) {
	h.t.Helper()
	h.call("POST", fmt.Sprintf("/api/runs/%d/commit", id), map[string]any{"confirm": "COMMIT", "allow_partial": true}, nil, want)
}
func TestAuthenticationCSRFOriginAndStaticIsolation(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("GET", "/api/me", nil, nil, 200)
	h.call("POST", "/api/setup", map[string]string{"username": "second", "password": "another-safe-password"}, nil, 409)
	h.call("POST", "/api/records/departments", map[string]string{"code": "X", "name": "X"}, map[string]string{"X-ASFWMS-Request": ""}, 403)
	h.call("POST", "/api/records/departments", map[string]string{"code": "X", "name": "X"}, map[string]string{"X-CSRF-Token": "wrong"}, 403)
	h.call("POST", "/api/records/departments", map[string]string{"code": "X", "name": "X"}, map[string]string{"Origin": "https://foreign.example"}, 403)
	h.call("GET", "/api/status", nil, map[string]string{"Host": "public.example"}, 200)
	for _, path := range []string{"/database/database.db", "/.env", "/server/main.go", "/scripts/../../database/database.db"} {
		h.call("GET", path, nil, nil, 404)
	}
	h.call("POST", "/api/logout", map[string]any{}, nil, 200)
	h.call("GET", "/api/bootstrap", nil, nil, 401)
	h.call("POST", "/api/login", map[string]string{"username": "test_admin", "password": "wrong-password"}, nil, 401)
	h.call("POST", "/api/login", map[string]string{"username": "test_admin", "password": "ASFWMS-test-password-42"}, nil, 200)
}
func TestCRUDValidationStaleEditAndAdviserIndependence(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	rev, _ := storage.Revision(context.Background(), h.store.DB)
	h.call("PUT", "/api/sections/1", map[string]any{}, nil, 405)
	h.call("PUT", "/api/records/sections/1", map[string]any{"adviser_id": 2}, map[string]string{"If-Match": fmt.Sprint(rev)}, 200)
	var instructor int
	h.store.DB.QueryRow("SELECT faculty_id FROM teaching_assignments WHERE section_id=1").Scan(&instructor)
	if instructor != 1 {
		t.Fatal("adviser changed instructor")
	}
	h.call("PUT", "/api/records/sections/1", map[string]any{"size": 35}, map[string]string{"If-Match": fmt.Sprint(rev)}, 409)
	h.call("PUT", "/api/records/sections/1", map[string]any{"curriculum_id": 2}, nil, 409)
	h.call("POST", "/api/records/subjects", map[string]any{"code": "BAD", "title": "Bad", "weekly_minutes": 61}, nil, 409)
	h.call("POST", "/api/records/departments", map[string]any{"code": "X", "name": "X", "admin": true}, nil, 409)
	h.call("DELETE", "/api/records/programs/1", map[string]string{"confirm": "DELETE"}, nil, 409)
	if h.count("programs") != 2 {
		t.Fatal("referenced program deleted")
	}
}
func TestPreviewCommitProtectionReplacementAndRelease(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	id := h.generate(1)
	if h.count("scheduled_meetings") != 0 {
		t.Fatal("preview reserved rooms")
	}
	h.commit(id, 200)
	if h.count("scheduled_meetings") != 3 {
		t.Fatal("commit failed")
	}
	h.commit(id, 200)
	if h.count("scheduled_meetings") != 3 {
		t.Fatal("duplicate commit created duplicates")
	}
	other := h.generate(2)
	h.commit(other, 200)
	if h.count("scheduled_meetings") != 6 {
		t.Fatal("second department lost meetings")
	}
	h.call("PUT", "/api/records/subjects/1", map[string]any{"weekly_minutes": 240}, nil, 409)
	h.call("POST", "/api/faculty/1/work-times", map[string]any{"use_default": true}, nil, 409)
	next := h.generate(1)
	h.commit(next, 200)
	if h.count("scheduled_meetings") != 6 {
		t.Fatal("replacement erased another department")
	}
	var status string
	h.store.DB.QueryRow("SELECT status FROM generation_runs WHERE id=?", id).Scan(&status)
	if status != "replaced" {
		t.Fatal(status)
	}
	h.call("POST", fmt.Sprintf("/api/runs/%d/release", next), map[string]string{"confirm": "RELEASE"}, nil, 200)
	if h.count("scheduled_meetings") != 3 {
		t.Fatal("release affected other department")
	}
	h.commit(id, 409)
	h.call("PUT", "/api/records/subjects/1", map[string]any{"weekly_minutes": 240}, nil, 200)
}
func TestStalePreviewRejectedWithNoSideEffects(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	id := h.generate(1)
	h.call("PUT", "/api/records/faculty/3", map[string]any{"name": "Changed adviser"}, nil, 200)
	h.commit(id, 409)
	if h.count("scheduled_meetings") != 0 {
		t.Fatal("stale preview changed reservations")
	}
}
func TestConcurrentPreviewCommitsAllowOneWinner(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	one, two := h.generate(1), h.generate(1)
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, id := range []int{one, two} {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			payload := strings.NewReader(`{"confirm":"COMMIT","allow_partial":true}`)
			req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/runs/%d/commit", h.server.URL, id), payload)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-ASFWMS-Request", "1")
			req.Header.Set("X-CSRF-Token", h.csrf)
			res, err := h.client.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			statuses <- res.StatusCode
		}(id)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 || h.count("scheduled_meetings") != 3 {
		t.Fatal(counts, "concurrent commit was not isolated")
	}
}
func TestPartialRequiresAcknowledgmentAndBackupIsSQLite(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	_, err := h.store.DB.Exec("INSERT INTO subjects(id,code,title) VALUES(3,'MISSING','Missing instructor'); INSERT INTO curriculum_subject_years(curriculum_id,subject_id,year_level) VALUES(1,3,1)")
	if err != nil {
		t.Fatal(err)
	}
	id := h.generate(1)
	h.call("POST", fmt.Sprintf("/api/runs/%d/commit", id), map[string]any{"confirm": "COMMIT"}, nil, 409)
	if h.count("scheduled_meetings") != 0 {
		t.Fatal("unacknowledged partial committed")
	}
	h.commit(id, 200)
	res, body := h.call("POST", "/api/backup", map[string]any{}, nil, 200)
	if !bytes.HasPrefix(body, []byte("SQLite format 3")) || !strings.Contains(res.Header.Get("Content-Disposition"), ".db") {
		t.Fatal("invalid database backup")
	}
}
func TestBulkAssignmentAtomicAndWorkTimeReplace(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	h.call("POST", "/api/assignments", map[string]any{"faculty_id": 2, "subject_id": 1, "section_ids": []int{1}}, nil, 409)
	h.call("DELETE", "/api/records/teaching_assignments/1", map[string]string{"confirm": "DELETE"}, nil, 200)
	h.call("POST", "/api/assignments", map[string]any{"faculty_id": 2, "subject_id": 1, "section_ids": []int{1, 2}}, nil, 409)
	if h.count("teaching_assignments") != 1 {
		t.Fatal("bulk operation partially committed")
	}
	h.call("POST", "/api/assignments", map[string]any{"faculty_id": 2, "subject_id": 1, "section_ids": []int{1, 1}}, nil, 200)
	if h.count("teaching_assignments") != 2 {
		t.Fatal("deduplication failed")
	}
	h.call("POST", "/api/faculty/1/work-times", map[string]any{"days": []any{map[string]int{"day": 1, "start_minute": 480, "end_minute": 600}, map[string]int{"day": 3, "start_minute": 480, "end_minute": 600}}}, nil, 200)
	if h.count("faculty_work_times") != 2 {
		t.Fatal("work times not saved")
	}
	h.call("POST", "/api/faculty/1/work-times", map[string]any{"days": []any{}}, nil, 400)
	if h.count("faculty_work_times") != 2 {
		t.Fatal("invalid availability deleted data")
	}
	h.call("POST", "/api/faculty/1/work-times", map[string]any{"use_default": true}, nil, 200)
	if h.count("faculty_work_times") != 0 {
		t.Fatal("explicit default failed")
	}
}
func TestSQLTriggerBlocksPhysicalOverlap(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	id := h.generate(1)
	h.commit(id, 200)
	_, err := h.store.DB.Exec(`INSERT INTO scheduled_meetings(run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed) SELECT run_id,department_id,period_id,section_id,subject_id,faculty_id,room_id,day,start_minute,end_minute,borrowed FROM scheduled_meetings LIMIT 1`)
	if err == nil {
		t.Fatal("direct duplicate bypassed trigger")
	}
	_, err = h.store.DB.Exec("UPDATE scheduled_meetings SET start_minute=start_minute+15")
	if err == nil {
		t.Fatal("committed mutation allowed")
	}
	if h.count("scheduled_meetings") != 3 {
		t.Fatal("invalid mutation altered reservations")
	}
}
func TestPasswordChangeRevokesSessions(t *testing.T) {
	h := newHarness(t)
	h.call("POST", "/api/account/password", map[string]string{"current": "ASFWMS-test-password-42", "new": "Changed-test-password-73"}, nil, 200)
	h.call("GET", "/api/me", nil, nil, 401)
	h.call("POST", "/api/login", map[string]string{"username": "test_admin", "password": "ASFWMS-test-password-42"}, nil, 401)
	h.call("POST", "/api/login", map[string]string{"username": "test_admin", "password": "Changed-test-password-73"}, nil, 200)
}
