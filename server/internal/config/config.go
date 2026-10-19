// Package config resolves every application path from one explicit project root.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct{ Root, Host, Port, DBPath, PublicURL, SMTPEmail, SMTPPass, SMTPHost, SMTPPort string }

func Load(root string) (Config, error) {
	if root == "" {
		root = os.Getenv("ASFWMS_ROOT")
	}
	if root == "" {
		cwd, _ := os.Getwd()
		exe, _ := os.Executable()
		for _, base := range []string{cwd, filepath.Dir(exe)} {
			for d := base; ; d = filepath.Dir(d) {
				if st, err := os.Stat(filepath.Join(d, "database", "schema.sql")); err == nil && !st.IsDir() {
					root = d
					break
				}
				if d == filepath.Dir(d) {
					break
				}
			}
			if root != "" {
				break
			}
		}
	}
	if root == "" {
		return Config{}, errors.New("cannot find scheduler root; run from scheduler/server or pass -root PATH")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Config{}, err
	}
	values := map[string]string{}
	f, err := os.Open(filepath.Join(root, ".env"))
	if err != nil {
		return Config{}, fmt.Errorf("read root .env: %w", err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			return Config{}, fmt.Errorf("invalid .env line: expected KEY=value")
		}
		values[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), "\"'")
	}
	if err := scan.Err(); err != nil {
		return Config{}, err
	}
	get := func(k, d string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		if v := values[k]; v != "" {
			return v
		}
		return d
	}
	host, port := get("APP_HOST", "127.0.0.1"), get("APP_PORT", "20000")
	// The listener can bind to a specific interface or all interfaces. HTTPS
	// tunnel connectors can still target 127.0.0.1 even with APP_HOST=0.0.0.0.
	if host != "localhost" && net.ParseIP(host) == nil {
		return Config{}, errors.New("APP_HOST must be localhost or an IP address (use 0.0.0.0 for all IPv4 interfaces)")
	}
	publicURL := strings.TrimRight(get("APP_PUBLIC_URL", ""), "/")
	if publicURL != "" {
		u, e := url.Parse(publicURL)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return Config{}, errors.New("APP_PUBLIC_URL must be an HTTPS origin, such as https://asfwms.example.com (no path)")
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return Config{}, errors.New("APP_PORT must be 1024-65535")
	}
	if get("DB_DRIVER", "sqlite") != "sqlite" {
		return Config{}, errors.New("only SQLite is implemented; PostgreSQL migration is not automatic")
	}
	p := get("DB_PATH", "database/database.db")
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	rel, err := filepath.Rel(filepath.Join(root, "database"), p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Config{}, errors.New("DB_PATH must remain inside scheduler/database")
	}
	return Config{Root: root, Host: host, Port: port, DBPath: p, PublicURL: publicURL, SMTPEmail: get("SMTP_EMAIL", ""), SMTPPass: get("SMTP_PASS", ""), SMTPHost: get("SMTP_HOST", "smtp.gmail.com"), SMTPPort: get("SMTP_PORT", "587")}, nil
}
