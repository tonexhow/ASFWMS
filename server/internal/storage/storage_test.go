package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func schemaPath(t *testing.T) string {
	t.Helper()
	_, p, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(p), "..", "..", "..", "database", "schema.sql")
}
func TestSeedRestartAndIntegrity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "database.db")
	s, e := Open(p, schemaPath(t))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Seed(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = s.Seed(context.Background()); e == nil {
		t.Fatal("seed overwrote existing data")
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM subjects").Scan(&n)
	if n != 309 {
		t.Fatal(n)
	}
	rs, e := ReadRows(context.Background(), s.DB, "PRAGMA foreign_key_check")
	if e != nil || len(rs) != 0 {
		t.Fatal(rs, e)
	}
	s.Close()
	s, e = Open(p, schemaPath(t))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.DB.QueryRow("SELECT count(*) FROM subjects").Scan(&n)
	if n != 309 {
		t.Fatal("restart lost data")
	}
}
func TestRollbackAndNoOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "database.db")
	s, e := Open(p, schemaPath(t))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	e = s.Write(context.Background(), func(q Querier) error {
		q.ExecContext(context.Background(), "INSERT INTO departments(id,code,name) VALUES(1,'X','X')")
		_, e := q.ExecContext(context.Background(), "INSERT INTO programs(department_id,code,name) VALUES(999,'X','X')")
		return e
	})
	if e == nil {
		t.Fatal("invalid FK accepted")
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM departments").Scan(&n)
	if n != 0 {
		t.Fatal("transaction partly committed")
	}
	_, e = s.DB.Exec("UPDATE schema_migrations SET number=99 WHERE number=(SELECT max(number) FROM schema_migrations)")
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	if _, e = Open(p, schemaPath(t)); e == nil {
		t.Fatal("unrecognized schema accepted")
	}
	if st, e := os.Stat(p); e != nil || st.Size() == 0 {
		t.Fatal("database destroyed")
	}
}
