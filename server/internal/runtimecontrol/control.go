// Package runtimecontrol provides per-process logs and a loopback-only,
// capability-protected graceful shutdown for the windowless desktop binaries.
package runtimecontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const stateName = ".running.json"

type Session struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
	StartedAt string `json:"started_at"`
	LogFile   string `json:"log_file"`
}

func DiscoverRoot() (string, error) {
	candidates := []string{}
	if r := strings.TrimSpace(os.Getenv("ASFWMS_ROOT")); r != "" {
		candidates = append(candidates, r)
	}
	if cwd, e := os.Getwd(); e == nil {
		candidates = append(candidates, cwd)
	}
	if exe, e := os.Executable(); e == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	for _, base := range candidates {
		base, _ = filepath.Abs(base)
		for dir := base; ; dir = filepath.Dir(dir) {
			if info, e := os.Stat(filepath.Join(dir, "database", "schema.sql")); e == nil && !info.IsDir() {
				return dir, nil
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", errors.New("ASFWMS project root not found; expected database/schema.sql beside .env")
}

func NewLog(root string) (*os.File, error) {
	dir := filepath.Join(root, "logs")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	// Timestamp + PID + cryptographic suffix prevents collisions between runs.
	nonce := make([]byte, 4)
	if _, e := rand.Read(nonce); e != nil {
		return nil, e
	}
	filename := fmt.Sprintf("asfwms_%s_pid%d_%s.log", time.Now().Format("2006-01-02_15-04-05.000"), os.Getpid(), hex.EncodeToString(nonce))
	return os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}

func statePath(root string) string { return filepath.Join(root, "logs", stateName) }

func Read(root string) (Session, error) {
	b, e := os.ReadFile(statePath(root))
	if e != nil {
		return Session{}, e
	}
	var s Session
	if e = json.Unmarshal(b, &s); e != nil {
		return Session{}, e
	}
	if s.Port < 1 || s.Port > 65535 || len(s.Token) != 64 || s.PID <= 0 {
		return Session{}, errors.New("invalid running-session file")
	}
	return s, nil
}

func SendStop(root string) error {
	s, e := Read(root)
	if e != nil {
		return fmt.Errorf("no active ASFWMS process: %w", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/shutdown", s.Port), bytes.NewReader(nil))
	if e != nil {
		return e
	}
	req.Header.Set("X-ASFWMS-Control", s.Token)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("ASFWMS is not responding; it may already have stopped: %w", e)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
	if res.StatusCode != http.StatusAccepted {
		return fmt.Errorf("shutdown refused (%s)", res.Status)
	}
	return nil
}

// ServeControl must be called only after the main application's network
// listener is available. The secret is never exposed through the web app.
func ServeControl(root, logFile string, shutdown func()) (func(), error) {
	if e := os.MkdirAll(filepath.Join(root, "logs"), 0700); e != nil {
		return nil, e
	}
	path := statePath(root)
	// Avoid deleting the runtime file of a live instance.
	if old, e := Read(root); e == nil {
		if alive(old) {
			return nil, fmt.Errorf("ASFWMS already running as PID %d", old.PID)
		}
	}
	// A current incomplete state may mean another instance is starting; only
	// recover it after a short delay rather than racing with its initial write.
	if info, e := os.Stat(path); e == nil && time.Since(info.ModTime()) < 3*time.Second {
		return nil, errors.New("another ASFWMS instance may be starting; try again shortly")
	}
	// Stale state is not an authority; the control listener still needs its token.
	_ = os.Remove(path)
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		listener.Close()
		return nil, e
	}
	s := Session{PID: os.Getpid(), Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(token), StartedAt: time.Now().Format(time.RFC3339), LogFile: filepath.Base(logFile)}
	b, _ := json.MarshalIndent(s, "", "  ")
	b = append(b, '\n')
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		listener.Close()
		return nil, e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		listener.Close()
		os.Remove(path)
		return nil, e
	}
	if e = f.Close(); e != nil {
		listener.Close()
		os.Remove(path)
		return nil, e
	}
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		provided := r.Header.Get("X-ASFWMS-Control")
		if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() || subtle.ConstantTimeCompare([]byte(provided), []byte(s.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Stopping ASFWMS"))
		once.Do(func() { go shutdown() })
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go server.Serve(listener)
	return func() {
		_ = server.Close()
		// Do not remove another process's state if files were changed externally.
		if current, err := Read(root); err == nil && current.PID == s.PID && current.Token == s.Token {
			_ = os.Remove(path)
		}
	}, nil
}

func alive(s Session) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/shutdown", s.Port), nil)
	if e != nil {
		return false
	}
	res, e := (&http.Client{Timeout: 400 * time.Millisecond}).Do(req)
	if e != nil {
		return false
	}
	res.Body.Close()
	// The private server returns 405 for GET; any answer indicates a listener.
	return res.StatusCode == http.StatusMethodNotAllowed || res.StatusCode == http.StatusForbidden
}
