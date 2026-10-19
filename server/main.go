package main

import (
	"asfwms.local/scheduler/internal/config"
	"asfwms.local/scheduler/internal/httpapi"
	"asfwms.local/scheduler/internal/runtimecontrol"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"
)

func main() {
	root := flag.String("root", "", "project root (normally detected automatically)")
	seed := flag.Bool("seed", false, "load reference dataset into an empty database, then exit")
	check := flag.Bool("check", false, "verify SQLite integrity, then exit")
	audit := flag.Bool("audit-facilities", false, "report missing buildings and duplicate physical rooms")
	flag.Parse()
	// Log even configuration failures for GUI-subsystem executables: Windows
	// intentionally has no visible terminal to display them.
	logRoot := *root
	if logRoot == "" {
		var findErr error
		logRoot, findErr = runtimecontrol.DiscoverRoot()
		if findErr != nil {
			log.Fatal(findErr)
		}
	}
	// Every application run gets an independent dated log. Diagnostic one-shot
	// commands keep printing to the terminal, and also write their log.
	logFile, err := runtimecontrol.NewLog(logRoot)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(logFile, os.Stderr))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	defer func() {
		if v := recover(); v != nil {
			log.Printf("FATAL PANIC: %v\n%s", v, debug.Stack())
			panic(v)
		}
	}()
	log.Printf("ASFWMS session started, PID=%d, log=%s", os.Getpid(), logFile.Name())
	defer log.Printf("ASFWMS session ended, PID=%d", os.Getpid())
	cfg, err := config.Load(logRoot)
	if err != nil {
		log.Fatal(err)
	}

	s, err := storage.Open(cfg.DBPath, filepath.Join(cfg.Root, "database", "schema.sql"))
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if *seed {
		if err = s.Seed(context.Background()); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Reference data loaded into an empty SQLite database; existing data was not overwritten.")
		return
	}
	if *check {
		var status string
		if err = s.DB.QueryRow("PRAGMA integrity_check").Scan(&status); err != nil || status != "ok" {
			log.Fatalf("integrity check: %s %v", status, err)
		}
		rs, e := storage.ReadRows(context.Background(), s.DB, "PRAGMA foreign_key_check")
		if e != nil || len(rs) > 0 {
			log.Fatalf("foreign key check failed: %v (%d rows)", e, len(rs))
		}
		fmt.Println("SQLite integrity: ok; foreign keys: ok")
		return
	}
	if *audit {
		issues, e := storage.AuditFacilities(s.DB)
		if e != nil {
			log.Fatal(e)
		}
		if len(issues) == 0 {
			fmt.Println("Facility audit: no broken references or duplicate active physical rooms found")
		}
		for _, v := range issues {
			fmt.Printf("%s: %s\n", v.Code, v.Message)
		}
		return
	}

	// Listen before announcing a running state. Failure must leave an explicit
	// diagnostic rather than a silent Windows GUI process.
	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		log.Fatalf("ASFWMS failed to bind %s:%s: %v", cfg.Host, cfg.Port, err)
	}
	defer listener.Close()
	srv := &http.Server{Handler: httpapi.New(s, cfg).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Only a random local control port can shut down the windowless server.
	cleanControl, err := runtimecontrol.ServeControl(cfg.Root, logFile.Name(), cancel)
	if err != nil {
		log.Fatalf("ASFWMS cannot register running instance: %v", err)
	}
	defer cleanControl()
	log.Printf("Listening on %s; open http://127.0.0.1:%s", listener.Addr().String(), cfg.Port)
	if cfg.PublicURL != "" {
		log.Printf("Public origin: %s", cfg.PublicURL)
	}
	if cfg.Host == "0.0.0.0" || cfg.Host == "::" {
		log.Printf("Security notice: bound to all interfaces; restrict via firewall or use APP_HOST=127.0.0.1 for tunnel-only access")
	}
	log.Printf("SQLite database: %s", cfg.DBPath)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(listener) }()
	select {
	case err = <-errc:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ASFWMS HTTP server stopped unexpectedly: %v", err)
		}
	case <-stop.Done():
		log.Printf("Graceful shutdown requested")
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		if err = srv.Shutdown(ctx); err != nil {
			log.Printf("Shutdown warning: %v", err)
		}
		c()
	}
}
