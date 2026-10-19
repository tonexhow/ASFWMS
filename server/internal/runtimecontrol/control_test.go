package runtimecontrol

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGracefulShutdown(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "database"), 0700)
	_ = os.WriteFile(filepath.Join(root, "database", "schema.sql"), []byte("-- test"), 0600)
	log, e := NewLog(root)
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	if !strings.HasSuffix(log.Name(), ".log") {
		t.Fatal(log.Name())
	}
	var calls atomic.Int32
	done := make(chan struct{})
	clean, e := ServeControl(root, log.Name(), func() { calls.Add(1); close(done) })
	if e != nil {
		t.Fatal(e)
	}
	defer clean()
	if _, e = ServeControl(root, log.Name(), func() {}); e == nil {
		t.Fatal("second instance incorrectly allowed")
	}
	if e = SendStop(root); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown was not delivered")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected calls %d", calls.Load())
	}
	clean()
	if _, e = Read(root); e == nil {
		t.Fatal("running file retained after stop")
	}
}
