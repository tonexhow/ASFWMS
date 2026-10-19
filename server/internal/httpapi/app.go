package httpapi

import (
	"asfwms.local/scheduler/internal/config"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	Store     *storage.Store
	Config    config.Config
	waitMu    sync.Mutex
	limiterMu sync.Mutex
	attempts  map[string]attempt
}
type attempt struct {
	Count int
	Reset time.Time
}
type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	DepartmentID int    `json:"department_id"`
	Protected    bool   `json:"protected"`
	CSRF         string `json:"csrf_token"`
}
type userKey struct{}

func New(s *storage.Store, c config.Config) *App {
	return &App{Store: s, Config: c, attempts: map[string]attempt{}}
}
func (a *App) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/status", a.status)
	m.HandleFunc("POST /api/setup", a.setup)
	m.HandleFunc("POST /api/login", a.login)
	m.Handle("GET /api/me", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, user(r)) })))
	m.Handle("POST /api/logout", a.auth(http.HandlerFunc(a.logout)))
	m.Handle("POST /api/account/password", a.auth(http.HandlerFunc(a.password)))
	m.Handle("GET /api/access/users", a.auth(http.HandlerFunc(a.accessUsers)))
	m.Handle("GET /api/access/reset-requests", a.auth(http.HandlerFunc(a.accessResetRequests)))
	m.Handle("POST /api/access/users", a.auth(http.HandlerFunc(a.accessCreate)))
	m.Handle("PUT /api/access/users/{id}", a.auth(http.HandlerFunc(a.accessUpdate)))
	m.Handle("POST /api/access/users/{id}/reset-email", a.auth(http.HandlerFunc(a.resetEmail)))
	m.Handle("POST /api/access/users/{id}/disable", a.auth(http.HandlerFunc(a.accessDisable)))
	m.Handle("POST /api/account/reset-request", a.auth(http.HandlerFunc(a.requestReset)))
	m.HandleFunc("POST /api/reset/confirm", a.resetConfirm)
	m.Handle("GET /api/developer/database", a.auth(http.HandlerFunc(a.devDatabase)))
	m.Handle("POST /api/developer/database", a.auth(http.HandlerFunc(a.devDatabaseWrite)))
	m.Handle("POST /api/developer/database/bulk-preview", a.auth(http.HandlerFunc(a.devBulkPreview)))
	m.Handle("POST /api/developer/database/bulk-delete", a.auth(http.HandlerFunc(a.devBulkDelete)))
	m.Handle("GET /api/developer/system", a.auth(http.HandlerFunc(a.devSystem)))
	m.Handle("POST /api/developer/clear-schedule", a.auth(http.HandlerFunc(a.devClearSchedule)))
	m.Handle("GET /api/timetable/pdf", a.auth(http.HandlerFunc(a.schedulePDF)))
	m.Handle("GET /api/timetable/zip", a.auth(http.HandlerFunc(a.scheduleZIP)))
	m.Handle("POST /api/timetable/faculty/email", a.auth(http.HandlerFunc(a.emailFacultyTimetable)))
	m.Handle("GET /api/records/{kind}/{id}/dependencies", a.auth(http.HandlerFunc(a.recordDependencies)))
	m.Handle("POST /api/records/{kind}/{id}/move-dependencies", a.auth(http.HandlerFunc(a.moveLinkedSections)))
	m.Handle("GET /api/bootstrap", a.auth(http.HandlerFunc(a.bootstrap)))
	m.Handle("GET /api/manifest", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(storage.ManifestJSON)
	})))
	m.Handle("POST /api/records/{kind}", a.auth(http.HandlerFunc(a.createRecord)))
	m.Handle("PUT /api/records/{kind}/{id}", a.auth(http.HandlerFunc(a.updateRecord)))
	m.Handle("DELETE /api/records/{kind}/{id}", a.auth(http.HandlerFunc(a.deleteRecord)))
	m.Handle("POST /api/assignments", a.auth(http.HandlerFunc(a.assign)))
	m.Handle("POST /api/assignments/reassign", a.auth(http.HandlerFunc(a.reassign)))
	m.Handle("POST /api/faculty/{id}/sections", a.auth(http.HandlerFunc(a.facultySectionSelections)))
	m.Handle("POST /api/faculty/{id}/section-policy", a.auth(http.HandlerFunc(a.sectionPolicy)))
	m.Handle("POST /api/faculty/{id}/work-times", a.auth(http.HandlerFunc(a.workTimes)))
	m.Handle("POST /api/faculty/{id}/preferences", a.auth(http.HandlerFunc(a.facultyPreferences)))
	m.Handle("POST /api/developer/sections/batch", a.auth(http.HandlerFunc(a.developerBatchSections)))
	m.Handle("POST /api/faculty/{id}/subjects", a.auth(http.HandlerFunc(a.facultySubjects)))
	m.Handle("POST /api/subjects/{id}/faculty", a.auth(http.HandlerFunc(a.subjectFaculty)))
	m.Handle("POST /api/curricula/{id}/members", a.auth(http.HandlerFunc(a.curriculumMembers)))
	m.Handle("POST /api/readiness", a.auth(http.HandlerFunc(a.readiness)))
	m.Handle("POST /api/generate", a.auth(http.HandlerFunc(a.generate)))
	m.Handle("POST /api/generate/stream", a.auth(http.HandlerFunc(a.generate)))
	m.Handle("GET /api/waits", a.auth(http.HandlerFunc(a.listWaits)))
	m.Handle("POST /api/waits/{id}/cancel", a.auth(http.HandlerFunc(a.cancelWait)))
	m.Handle("POST /api/runs/{id}/email", a.auth(http.HandlerFunc(a.scheduleEmail)))
	m.Handle("POST /api/runs/{id}/wait", a.auth(http.HandlerFunc(a.waitForFacilities)))
	m.Handle("GET /api/runs/{id}/facilities", a.auth(http.HandlerFunc(a.facilityCandidates)))
	m.Handle("GET /api/runs/{id}", a.auth(http.HandlerFunc(a.getRun)))
	m.Handle("POST /api/runs/{id}/commit", a.auth(http.HandlerFunc(a.commit)))
	m.Handle("POST /api/runs/{id}/release", a.auth(http.HandlerFunc(a.release)))
	m.Handle("POST /api/backup", a.auth(http.HandlerFunc(a.backup)))
	m.Handle("POST /api/seed", a.auth(http.HandlerFunc(a.seed)))
	for _, d := range []string{"styles", "scripts", "assets"} {
		dir := d
		m.Handle("GET /"+dir+"/", http.StripPrefix("/"+dir+"/", http.FileServer(http.Dir(filepath.Join(a.Config.Root, "client", dir)))))
	}
	for path, file := range map[string]string{"/": "index.html", "/access": "access.html", "/guide": "guide.html", "/print": "print.html"} {
		p, f := path, file
		m.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != p {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(a.Config.Root, "client", "structures", f))
		})
	}
	return a.middleware(m)
}
func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("request panic: %v", v)
				problem(w, 500, "INTERNAL", "The request failed. No successful save was confirmed.")
			}
		}()
		// Do not reject the public Host used by an HTTPS tunnel. When a fixed
		// public origin is configured, reject other external Host values to
		// reduce Host-header poisoning and DNS-rebinding exposure.
		if !a.allowedHost(r) {
			problem(w, 403, "HOST", "This hostname is not configured for ASFWMS.")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method != "GET" && r.Method != "HEAD" {
				if r.Header.Get("X-ASFWMS-Request") != "1" {
					problem(w, 403, "REQUEST_HEADER", "Use the ASFWMS application to submit changes.")
					return
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					u, e := url.Parse(origin)
					if e != nil || u.Host != r.Host || u.Scheme != a.requestScheme(r) {
						problem(w, 403, "ORIGIN", "Cross-origin changes are not permitted.")
						return
					}
				}
				ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if ct != "application/json" {
					problem(w, 415, "CONTENT_TYPE", "Send application/json.")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requestScheme only trusts the forwarded protocol from a proxy connecting
// over the loopback interface. A Cloudflare Tunnel on the same machine is one
// such proxy; unauthenticated internet clients cannot spoof it remotely.
func (a *App) requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if isLoopbackPeer(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

func isLoopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func isLoopbackHostname(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func (a *App) allowedHost(r *http.Request) bool {
	if r.Host == "" || strings.ContainsAny(r.Host, " \t\r\n\\/") {
		return false
	}
	if a.Config.PublicURL == "" || isLoopbackHostname(r.Host) {
		return true
	}
	u, err := url.Parse(a.Config.PublicURL)
	return err == nil && strings.EqualFold(r.Host, u.Host)
}

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return fmt.Errorf("invalid JSON: %w", e)
	}
	var x any
	if e := d.Decode(&x); !errors.Is(e, io.EOF) {
		return errors.New("send exactly one JSON object")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if e := json.NewEncoder(w).Encode(v); e != nil {
		log.Printf("response encoding: %v", e)
	}
}
func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}
func idOf(r *http.Request) (int, error) {
	n, e := strconv.Atoi(r.PathValue("id"))
	if e != nil || n < 1 {
		return 0, errors.New("invalid record id")
	}
	return n, nil
}
func user(r *http.Request) User { u, _ := r.Context().Value(userKey{}).(User); return u }
func audit(ctx context.Context, q storage.Querier, u User, action, entity string, id int, detail string) error {
	_, e := q.ExecContext(ctx, "INSERT INTO audit_events(user_id,action,entity,entity_id,detail) VALUES(?,?,?,?,?)", u.ID, action, entity, id, detail)
	return e
}
func safeError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint"):
		return "That code or relationship already exists. A subject-section can have only one instructor."
	case strings.Contains(msg, "FOREIGN KEY constraint"):
		return "This record is referenced by other records, or a selected record no longer exists. Remove or correct the related links first."
	case strings.Contains(msg, "CHECK constraint"):
		return "One or more values violate the database validation rules."
	case strings.Contains(msg, "locked"):
		return "The database is busy. Wait briefly and retry."
	}
	return msg
}
func (a *App) backup(w http.ResponseWriter, r *http.Request) {
	f, e := os.CreateTemp("", "asfwms-backup-*.db")
	if e != nil {
		problem(w, 500, "BACKUP", "Cannot create temporary backup.")
		return
	}
	path := f.Name()
	f.Close()
	os.Remove(path)
	defer os.Remove(path)
	if _, e = a.Store.DB.ExecContext(r.Context(), "VACUUM INTO ?", path); e != nil {
		problem(w, 500, "BACKUP", safeError(e))
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=ASFWMS_backup_"+time.Now().Format("2006-01-02_150405")+".db")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}
func (a *App) seed(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if e := decode(r, &in); e != nil || in.Confirm != "LOAD REFERENCE" {
		problem(w, 400, "CONFIRM", "Type LOAD REFERENCE to confirm.")
		return
	}
	if e := a.Store.Seed(r.Context()); e != nil {
		problem(w, 409, "SEED_REFUSED", safeError(e))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
