package httpapi

import (
	"asfwms.local/scheduler/internal/security"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"net/smtp"
	"regexp"
	"strings"
	"time"
)

var mailAddress = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type accessInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	DepartmentID int    `json:"department_id"`
	Active       *bool  `json:"active,omitempty"`
}

func accessValid(u User, x accessInput, create bool) error {
	x.Role = strings.ToLower(strings.TrimSpace(x.Role))
	if x.Role != "developer" && x.Role != "administrator" && x.Role != "scheduler" {
		return errors.New("choose Developer, Administrator, or departmental Scheduler")
	}
	if x.Role == "developer" && u.Role != "developer" {
		return errors.New("only a Developer can grant Developer access")
	}
	if !validUsername(strings.TrimSpace(x.Username)) {
		return errors.New("username must contain 3-50 letters, digits, dots, hyphens or underscores")
	}
	if len(strings.TrimSpace(x.Name)) < 2 || len(x.Name) > 140 {
		return errors.New("enter a name (2-140 characters)")
	}
	if len(x.Email) > 254 || !mailAddress.MatchString(x.Email) {
		return errors.New("enter a valid email address")
	}
	if _, err := mail.ParseAddress(x.Email); err != nil {
		return errors.New("enter a valid email address")
	}
	if x.Role == "scheduler" && x.DepartmentID < 1 {
		return errors.New("a Scheduler must have a department")
	}
	if create && x.Password == "" {
		return errors.New("enter an initial password (12+ characters)")
	}
	return nil
}
func (a *App) accessUsers(w http.ResponseWriter, r *http.Request) {
	data, err := storage.ReadRows(r.Context(), a.Store.DB, `SELECT id,name,email,username,role,COALESCE(department_id,0) department_id,protected,active,created_at FROM users ORDER BY protected DESC,role,username`)
	if err != nil {
		problem(w, 500, "USERS", "Cannot load accounts.")
		return
	}
	// Preserve the account identity on the owner's screen; other developers
	// receive no private account details for the protected primary developer.
	for _, item := range data {
		if domainInt(item, "protected") == 1 && domainInt(item, "id") != user(r).ID {
			item["name"] = "Protected primary Developer"
			item["username"] = "(private)"
			item["email"] = ""
		}
	}
	writeJSON(w, 200, map[string]any{"users": data})
}
func domainInt(m map[string]any, key string) int {
	switch n := m[key].(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
func (a *App) accessResetRequests(w http.ResponseWriter, r *http.Request) {
	entries, err := storage.ReadRows(r.Context(), a.Store.DB, `SELECT a.id,a.user_id,u.username,u.name,a.created_at,a.detail FROM audit_events a JOIN users u ON u.id=a.user_id WHERE a.action='reset-request' AND u.active=1 AND u.protected=0 ORDER BY a.id DESC LIMIT 40`)
	if err != nil {
		problem(w, 500, "RESET_REQUESTS", "Cannot load recovery requests.")
		return
	}
	writeJSON(w, 200, map[string]any{"requests": entries})
}
func (a *App) accessCreate(w http.ResponseWriter, r *http.Request) {
	var in accessInput
	if err := decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if err := accessValid(user(r), in, true); err != nil {
		problem(w, 400, "VALIDATION", err.Error())
		return
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		problem(w, 400, "PASSWORD", err.Error())
		return
	}
	if in.Role != "scheduler" {
		in.DepartmentID = 0
	} else if !idHasDepartmentSelf(r.Context(), a.Store.DB, in.DepartmentID) {
		problem(w, 400, "DEPARTMENT", "Select an existing active department.")
		return
	}
	var id int64
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var dept any
		if in.DepartmentID > 0 {
			dept = in.DepartmentID
		}
		res, err := q.ExecContext(r.Context(), "INSERT INTO users(name,email,username,password_hash,role,department_id) VALUES(?,?,?,?,?,?)", in.Name, in.Email, in.Username, hash, in.Role, dept)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return audit(r.Context(), q, user(r), "create", "users", int(id), "Account created with role "+in.Role)
	})
	if err != nil {
		problem(w, 409, "CREATE", safeError(err))
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "ok": true})
}
func idHasDepartmentSelf(ctx context.Context, db *sql.DB, id int) bool {
	var n int
	return db.QueryRowContext(ctx, "SELECT COUNT(*) FROM departments WHERE id=? AND active=1", id).Scan(&n) == nil && n == 1
}
func (a *App) accessUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in accessInput
	if err = decode(r, &in); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if err = accessValid(user(r), in, false); err != nil {
		problem(w, 400, "VALIDATION", err.Error())
		return
	}
	var previousRole string
	var isProtected int
	if err = a.Store.DB.QueryRowContext(r.Context(), "SELECT role,protected FROM users WHERE id=?", id).Scan(&previousRole, &isProtected); err != nil {
		problem(w, 404, "USER", "Account not found.")
		return
	}
	if isProtected == 1 {
		if id != user(r).ID {
			problem(w, 403, "PROTECTED", "The original Developer account can only be edited by its owner.")
			return
		}
		if in.Role != "developer" || in.DepartmentID != 0 || (in.Active != nil && !*in.Active) {
			problem(w, 403, "PROTECTED", "Your primary Developer role and active status are protected. You may update your own name, email, or username.")
			return
		}
	}
	if previousRole == "developer" && user(r).Role != "developer" {
		problem(w, 403, "ROLE", "Only a Developer can edit another Developer account.")
		return
	}
	if id == user(r).ID && (in.Role != previousRole || (in.Active != nil && !*in.Active)) {
		problem(w, 403, "SELF", "You cannot change your own role or disable your signed-in account.")
		return
	}
	if in.Role != "scheduler" {
		in.DepartmentID = 0
	} else if !idHasDepartmentSelf(r.Context(), a.Store.DB, in.DepartmentID) {
		problem(w, 400, "DEPARTMENT", "Select an active department.")
		return
	}
	if in.Password != "" {
		problem(w, 400, "PASSWORD", "Passwords are never edited through Access. Use self-service change or an expiring reset email.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var dep any
		if in.DepartmentID > 0 {
			dep = in.DepartmentID
		}
		var active any = 1
		if in.Active != nil && !*in.Active {
			active = 0
		}
		_, err := q.ExecContext(r.Context(), "UPDATE users SET name=?,email=?,username=?,role=?,department_id=?,active=? WHERE id=? AND (protected=0 OR id=?)", in.Name, in.Email, in.Username, in.Role, dep, active, id, user(r).ID)
		if err != nil {
			return err
		}
		_, err = q.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", id)
		if err != nil {
			return err
		}
		return audit(r.Context(), q, user(r), "update", "users", id, "Account details and scope updated")
	})
	if err != nil {
		problem(w, 409, "UPDATE", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) accessDisable(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err = decode(r, &body); err != nil || body.Confirm != "DISABLE" {
		problem(w, 400, "CONFIRM", "Type DISABLE to revoke access.")
		return
	}
	if id == user(r).ID {
		problem(w, 403, "SELF", "You cannot disable your own account.")
		return
	}
	var role string
	var protected int
	if err = a.Store.DB.QueryRowContext(r.Context(), "SELECT role,protected FROM users WHERE id=?", id).Scan(&role, &protected); err != nil {
		problem(w, 404, "USER", "Account not found.")
		return
	}
	if protected != 0 || (role == "developer" && user(r).Role != "developer") {
		problem(w, 403, "PROTECTED", "This developer account cannot be disabled by your role.")
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		if _, e := q.ExecContext(r.Context(), "UPDATE users SET active=0 WHERE id=? AND protected=0", id); e != nil {
			return e
		}
		if _, e := q.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", id); e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "disable", "users", id, "Access revoked")
	})
	if err != nil {
		problem(w, 500, "DISABLE", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) requestReset(w http.ResponseWriter, r *http.Request) {
	// Do not reveal secrets or create temporary passwords.
	var body struct {
		Message string `json:"message"`
	}
	if err := decode(r, &body); err != nil {
		problem(w, 400, "INPUT", err.Error())
		return
	}
	if len(body.Message) > 500 {
		problem(w, 400, "INPUT", "Message is too long.")
		return
	}
	_, err := a.Store.DB.ExecContext(r.Context(), "INSERT INTO audit_events(user_id,action,entity,entity_id,detail) VALUES(?, 'reset-request','users',?,?)", user(r).ID, user(r).ID, strings.TrimSpace(body.Message))
	if err != nil {
		problem(w, 500, "REQUEST", "Unable to log your request.")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": "Password reset requested. An administrator can send a secure reset email."})
}
func (a *App) resetEmail(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err = decode(r, &in); err != nil || in.Confirm != "SEND" {
		problem(w, 400, "CONFIRM", "Type SEND to issue a reset link.")
		return
	}
	var email, role string
	var protected, active int
	if err = a.Store.DB.QueryRowContext(r.Context(), "SELECT email,role,protected,active FROM users WHERE id=?", id).Scan(&email, &role, &protected, &active); err != nil || active != 1 {
		problem(w, 404, "USER", "Active account not found.")
		return
	}
	if protected != 0 || (role == "developer" && user(r).Role != "developer") {
		problem(w, 403, "PROTECTED", "You cannot reset this Developer account.")
		return
	}
	if a.Config.SMTPEmail == "" || a.Config.SMTPPass == "" {
		problem(w, 503, "SMTP", "Set SMTP_EMAIL and SMTP_PASS in .env before sending recovery emails.")
		return
	}
	if !mailAddress.MatchString(email) {
		problem(w, 409, "EMAIL", "Set a valid recipient email in Access first.")
		return
	}
	token, err := security.Token()
	if err != nil {
		problem(w, 500, "RESET", "Could not create recovery token.")
		return
	}
	// Use a fixed HTTPS origin for public reset links; never build an email
	// destination from a freely supplied external Host header.
	baseURL := a.Config.PublicURL
	if baseURL == "" {
		if !isLoopbackHostname(r.Host) {
			problem(w, 503, "PUBLIC_URL", "Set APP_PUBLIC_URL to your HTTPS tunnel address before emailing a remote password-reset link.")
			return
		}
		baseURL = "http://" + r.Host
	}
	link := baseURL + "/access?reset=" + token
	body := "ASFWMS password recovery\r\n\r\nA developer/administrator issued a password reset for your account.\r\n\r\nOpen this link:\r\n" + link + "\r\n\r\nThe link expires in 30 minutes and works once. If you did not request it, ignore this email.\r\n"
	from := a.Config.SMTPEmail
	msg := "From: " + from + "\r\nTo: " + email + "\r\nSubject: ASFWMS password reset\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		_, e := q.ExecContext(r.Context(), "DELETE FROM password_reset_tokens WHERE user_id=?", id)
		if e != nil {
			return e
		}
		_, e = q.ExecContext(r.Context(), "INSERT INTO password_reset_tokens(token_hash,user_id,expires_at) VALUES(?,?,?)", security.Digest(token), id, time.Now().Add(30*time.Minute).Unix())
		if e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "reset-email-prepared", "users", id, "Recovery link generated; mail dispatch requested")
	})
	if err != nil {
		problem(w, 500, "RESET", "Could not prepare secure recovery token.")
		return
	}
	auth := smtp.PlainAuth("", from, a.Config.SMTPPass, "smtp.gmail.com")
	if err = smtp.SendMail("smtp.gmail.com:587", auth, from, []string{email}, []byte(msg)); err != nil {
		_ = a.Store.Write(r.Context(), func(q storage.Querier) error {
			_, e := q.ExecContext(r.Context(), "DELETE FROM password_reset_tokens WHERE token_hash=?", security.Digest(token))
			return e
		})
		problem(w, 502, "SMTP", "The SMTP provider refused delivery. Check Gmail SMTP_EMAIL and the App Password.")
		return
	}
	_ = a.Store.Write(r.Context(), func(q storage.Querier) error {
		return audit(r.Context(), q, user(r), "reset-email", "users", id, "Recovery email was sent")
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *App) resetConfirm(w http.ResponseWriter, r *http.Request) {
	if a.limited(r) {
		problem(w, 429, "RATE_LIMIT", "Too many attempts; wait before retrying.")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil || len(in.Token) != 64 {
		problem(w, 400, "RESET", "Invalid recovery request.")
		return
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		problem(w, 400, "PASSWORD", err.Error())
		return
	}
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		var id int
		var protected, used int
		var expires int64
		if err := q.QueryRowContext(r.Context(), "SELECT t.user_id,t.expires_at,t.used,u.protected FROM password_reset_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=?", security.Digest(in.Token)).Scan(&id, &expires, &used, &protected); err != nil {
			return errors.New("reset link is expired or invalid")
		}
		if protected != 0 || used != 0 || expires <= time.Now().Unix() {
			return errors.New("reset link is expired or invalid")
		}
		if _, err := q.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=? AND protected=0", hash, id); err != nil {
			return err
		}
		if _, err := q.ExecContext(r.Context(), "UPDATE password_reset_tokens SET used=1 WHERE token_hash=?", security.Digest(in.Token)); err != nil {
			return err
		}
		if _, err := q.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", id); err != nil {
			return err
		}
		return audit(r.Context(), q, User{ID: id}, "reset-confirmed", "users", id, "Password reset completed")
	})
	if err != nil {
		problem(w, 409, "RESET", "The reset link is expired or invalid.")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
