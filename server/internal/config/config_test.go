package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationRootPathsAndTunnelSafety(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "database"), 0700)
	os.WriteFile(filepath.Join(root, "database", "schema.sql"), []byte("-- test"), 0600)
	os.WriteFile(filepath.Join(root, ".env"), []byte("APP_HOST=127.0.0.1\nAPP_PORT=20000\nDB_PATH=database/database.db\n"), 0600)
	c, e := Load(root)
	if e != nil || c.DBPath != filepath.Join(root, "database", "database.db") {
		t.Fatal(c, e)
	}
	t.Setenv("APP_PORT", "20003")
	c, e = Load(root)
	if e != nil || c.Port != "20003" {
		t.Fatal("environment override", c, e)
	}
	t.Setenv("DB_PATH", "../outside.db")
	if _, e = Load(root); e == nil {
		t.Fatal("database outside root accepted")
	}
	t.Setenv("DB_PATH", "database/database.db")
	t.Setenv("APP_HOST", "0.0.0.0")
	c, e = Load(root)
	if e != nil || c.Host != "0.0.0.0" {
		t.Fatal("all-interface listener should be supported", c, e)
	}
	t.Setenv("APP_HOST", "bad/hostname")
	if _, e = Load(root); e == nil {
		t.Fatal("invalid APP_HOST accepted")
	}
	t.Setenv("APP_HOST", "127.0.0.1")
	t.Setenv("APP_PUBLIC_URL", "https://asfwms.example.com")
	c, e = Load(root)
	if e != nil || c.PublicURL != "https://asfwms.example.com" {
		t.Fatal("HTTPS tunnel URL not loaded", c, e)
	}
	t.Setenv("APP_PUBLIC_URL", "http://public.example.com")
	if _, e = Load(root); e == nil {
		t.Fatal("insecure public URL accepted")
	}
	t.Setenv("APP_PUBLIC_URL", "")
	t.Setenv("DB_DRIVER", "postgres")
	if _, e = Load(root); e == nil {
		t.Fatal("unimplemented driver accepted")
	}
}
