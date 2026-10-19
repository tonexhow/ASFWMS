package httpapi

import (
	"asfwms.local/scheduler/internal/security"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const cookieName = "asfwms_local_session"

func (a *App) status(w http.ResponseWriter, r *http.Request) {
	var n int
	if e := a.Store.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&n); e != nil {
		problem(w, 503, "DATABASE", "Local database is unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"product": "ASFWMS", "setup_required": n == 0, "local_only": false})
}
func (a *App) limited(r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.limiterMu.Lock()
	defer a.limiterMu.Unlock()
	v := a.attempts[ip]
	if time.Now().After(v.Reset) {
		v = attempt{Reset: time.Now().Add(time.Minute)}
	}
	v.Count++
	a.attempts[ip] = v
	return v.Count > 8
}
func validUsername(s string) bool {
	if len(s) < 3 || len(s) > 50 {
		return false
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	// First-run Developer registration must happen directly on the host.
	// A public tunnel forwards via loopback, so checking only RemoteAddr
	// would accidentally allow remote visitors to claim an empty database.
	if !isLoopbackHostname(r.Host) || !isLoopbackPeer(r) || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Proto") != "" || r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("Forwarded") != "" {
		problem(w, 403, "SETUP_LOCAL", fmt.Sprintf("Complete first-time setup at http://127.0.0.1:%s on the server computer before sharing a tunnel.", a.Config.Port))
		return
	}
	if a.limited(r) {
		problem(w, 429, "RATE_LIMIT", "Too many access attempts. Wait one minute.")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		problem(w, 400, "INPUT", e.Error())
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !validUsername(in.Username) {
		problem(w, 400, "USERNAME", "Use 3-50 letters, numbers, dots, hyphens or underscores.")
		return
	}
	hash, e := security.HashPassword(in.Password)
	if e != nil {
		problem(w, 400, "PASSWORD", e.Error())
		return
	}
	u := User{Username: in.Username}
	token, e := security.Token()
	if e != nil {
		problem(w, 500, "RANDOM", "Could not create a session.")
		return
	}
	u.CSRF, e = security.Token()
	if e != nil {
		problem(w, 500, "RANDOM", "Could not create a session.")
		return
	}
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var n int
		if e := q.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			return errors.New("setup is already complete; sign in with the local account")
		}
		res, e := q.ExecContext(r.Context(), "INSERT INTO users(username,password_hash,name,role,protected) VALUES(?,?,?,?,1)", in.Username, hash, in.Username, "developer")
		if e != nil {
			return e
		}
		id, e := res.LastInsertId()
		if e != nil {
			return e
		}
		u.ID = int(id)
		u.Name = in.Username
		u.Role = "developer"
		u.Protected = true
		return saveSession(r.Context(), q, token, u)
	})
	if e != nil {
		problem(w, 409, "SETUP", safeError(e))
		return
	}
	setCookie(w, r, token)
	writeJSON(w, 201, u)
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if a.limited(r) {
		problem(w, 429, "RATE_LIMIT", "Too many access attempts. Wait one minute.")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		problem(w, 400, "INPUT", e.Error())
		return
	}
	var u User
	var hash string
	e := a.Store.DB.QueryRowContext(r.Context(), "SELECT id,username,password_hash,name,email,role,COALESCE(department_id,0),protected FROM users WHERE username=? AND active=1", strings.TrimSpace(in.Username)).Scan(&u.ID, &u.Username, &hash, &u.Name, &u.Email, &u.Role, &u.DepartmentID, &u.Protected)
	if e != nil {
		security.PBKDF2SHA256([]byte("unavailable-account"), make([]byte, 16), security.Iterations)
		problem(w, 401, "CREDENTIALS", "The username or password is incorrect.")
		return
	}
	if !security.VerifyPassword(in.Password, hash) {
		problem(w, 401, "CREDENTIALS", "The username or password is incorrect.")
		return
	}
	token, e := security.Token()
	if e != nil {
		problem(w, 500, "SESSION", "Cannot create a session.")
		return
	}
	u.CSRF, e = security.Token()
	if e != nil {
		problem(w, 500, "SESSION", "Cannot create a session.")
		return
	}
	e = a.Store.Write(r.Context(), func(q storage.Querier) error { return saveSession(r.Context(), q, token, u) })
	if e != nil {
		problem(w, 500, "SESSION", "Cannot save the session.")
		return
	}
	setCookie(w, r, token)
	writeJSON(w, 200, u)
}
func saveSession(ctx context.Context, q storage.Querier, token string, u User) error {
	if _, e := q.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", time.Now().Unix()); e != nil {
		return e
	}
	_, e := q.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at) VALUES(?,?,?,?)", security.Digest(token), u.ID, u.CSRF, time.Now().Add(12*time.Hour).Unix())
	return e
}
func setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil || (isLoopbackPeer(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")), SameSite: http.SameSiteStrictMode, MaxAge: 43200})
}
func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(cookieName)
		if e != nil || len(c.Value) != 64 {
			problem(w, 401, "SIGN_IN", "Sign in to ASFWMS.")
			return
		}
		var u User
		e = a.Store.DB.QueryRowContext(r.Context(), "SELECT u.id,u.username,u.name,u.email,u.role,COALESCE(u.department_id,0),u.protected,s.csrf_token FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.active=1", security.Digest(c.Value), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.Name, &u.Email, &u.Role, &u.DepartmentID, &u.Protected, &u.CSRF)
		if e != nil {
			if !errors.Is(e, sql.ErrNoRows) {
				problem(w, 503, "DATABASE", "The session could not be verified.")
			} else {
				problem(w, 401, "SESSION_EXPIRED", "Your session ended. Sign in again.")
			}
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(u.CSRF)) != 1 {
			problem(w, 403, "CSRF", "Your security token changed. Reload this page and try again.")
			return
		}
		if !a.authorizeRoute(w, r, u) {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(cookieName)
	if c != nil {
		if _, e := a.Store.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", security.Digest(c.Value)); e != nil {
			problem(w, 500, "LOGOUT", "Could not revoke the session. Retry.")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) password(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if e := decode(r, &in); e != nil {
		problem(w, 400, "INPUT", e.Error())
		return
	}
	if a.limited(r) {
		problem(w, 429, "RATE_LIMIT", "Wait one minute before another password attempt.")
		return
	}
	var old string
	if e := a.Store.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=?", user(r).ID).Scan(&old); e != nil {
		problem(w, 500, "ACCOUNT", "Cannot read account.")
		return
	}
	if !security.VerifyPassword(in.Current, old) {
		problem(w, 403, "PASSWORD", "Current password is incorrect.")
		return
	}
	hash, e := security.HashPassword(in.New)
	if e != nil {
		problem(w, 400, "PASSWORD", e.Error())
		return
	}
	e = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if _, e := q.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=?", hash, user(r).ID); e != nil {
			return e
		}
		_, e := q.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", user(r).ID)
		return e
	})
	if e != nil {
		problem(w, 500, "PASSWORD", safeError(e))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
