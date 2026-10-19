# ASFWMS - Academic Scheduling & Faculty Workload Management System

Buenavista Community College | SQLite | Go backend | HTML/CSS/JavaScript frontend

## Folder layout

This repository contains **source code**, not private production credentials:

```text
asfwms/
  client/                      Frontend source/assets and timetable templates
  database/
    schema.sql                 Safe to commit; creates a NEW SQLite database
    database.db                Private working database (gitignored)
    seed.sql                   Private reference data (gitignored)
    backups/                   Original and migration backups (gitignored)
  server/
    main.go                    HTTP backend, logging, and graceful lifecycle
    cmd/stop/main.go           Independent stop application
    internal/                  Application / scheduler / storage
  systems/
    windows/                   Compiled silent .exe files after compile.bat
    macos/                     Compiled amd64 and arm64 files
    linux/                     Compiled amd64 and arm64 files
  logs/                        New dated log per server run, plus stop-actions.log
  .env                         Private configuration (gitignored)
  .env.example                 Blank public configuration template
  compile.bat                  Windows build script
  compile.sh                   macOS/Linux build script
  asfwms_run.bat               Development-only terminal runner
  dist/asfwms/                 Finished portable release; NOT for public GitHub
```

> SECURITY: This repository's `.gitignore` excludes `database.db`, seed.sql, backups, logs, `.env`, and built releases. Do not force-add those files or upload `dist/` to a public GitHub repository. A new clone can create its own empty SQLite database from `database/schema.sql` at first launch. Copy your real database into a **private** deployment only.

## Configure

1. Copy `.env.example` to `.env` if it is missing.
2. Set `APP_HOST=127.0.0.1` and `APP_PORT=3000` for a Cloudflare Tunnel that connects to localhost.
3. Set `APP_PUBLIC_URL=https://asfwms.bcc-edu.ph` **only** if this is your actual live HTTPS domain.
4. Keep `DB_DRIVER=sqlite` and `DB_PATH=database/database.db`.
5. Enter `SMTP_EMAIL` and `SMTP_PASS` only when email delivery is ready. Those are not required for printing or PDF downloads.

Do not commit `.env`. The provided private project ZIP has a local `.env` with the existing public URL; its SMTP secrets are blank.

## Development (requires Go 1.23+)

From `server/`:

```powershell
go test ./...
go vet ./...
go run . -check
go run . -audit-facilities
go run .
```

Open `http://127.0.0.1:3000`. The `-audit-facilities` command reports inactive/missing building references and possible room aliases without modifying records. `-check` checks SQLite integrity and foreign keys.

## Windows: compile and start without a terminal

1. Install Go and ensure `go version` works in PowerShell.
2. Run `compile.bat` from the source project root. Internet access may be needed **once** for Go dependencies.
3. After tests and compilation pass, open `dist/asfwms/`.
4. Double-click `systems/windows/asfwms_start.exe`. It runs without opening CMD/PowerShell.
5. Open `http://127.0.0.1:3000` in your browser (or use your configured Cloudflare public URL).
6. Double-click `systems/windows/asfwms_stop.exe` to request a **graceful shutdown**. It does not kill unrelated processes.

Each server launch creates `logs/asfwms_YYYY-MM-DD_HH-MM-SS.mmm_pid...log`. Read the newest log if the executable appears unresponsive. `logs/stop-actions.log` records stop requests or errors. The private `logs/.running.json` contains a random, loopback-only shutdown capability and is deleted on normal exit. Logs and control files must not be web-exposed.

`compile.bat` makes a self-contained `dist/asfwms/` distribution with the frontend, SQLite schema, local database if supplied, `.env`, and Windows/macOS/Linux binaries. Cross-platform binaries are independent; do not try to run an `.exe` on macOS/Linux. To compile directly on macOS/Linux, run `./compile.sh` from the repository root.

macOS binaries are **unsigned** when cross-compiled on Windows. Organization-managed distribution may require code signing/notarization; the architecture-specific executable must be marked executable on Unix (`chmod +x`).

## Database and facilities

The supplied SQLite snapshot contained physically duplicated CITE room records. For the initial finalized private distribution, seven legacy *alias* room records have been marked **inactive** (not deleted), keeping their IDs and the original database backed up in `database/backups/`. The scheduler additionally recognizes matching normalized room names inside the same building as a single occupied physical space, even when older references use an alias. The original data contains no committed meetings or preferences linked to those alias IDs.

There is **no building named CITE 1**. `CITE-1` was a legacy *room code* for IT Lab 1; the actual building is `College of Information Technology Bldg.`. On the Facilities screen, inactive aliases remain inspectable when the inactive filter is enabled.

## Deploying and testing

- Keep the local Go listener at `127.0.0.1` if Cloudflare Tunnel terminates public traffic; configure Cloudflare Tunnel to reach `http://127.0.0.1:3000`.
- Allow only intended users to reach the application. Complete the initial Developer setup **locally**, not over a public tunnel.
- Set a strong unique Developer password; review departmental moderator permissions. Public access must use authenticated HTTPS only.
- Verify the correct hostname in `.env`. Restart the process after editing `.env`.
- Test generation standard/force/wait, borrowing boundaries, semester changes, schedule previews, PDF printing, and SMTP delivery with your own accounts.
- Back up `database.db` regularly while the app is stopped or use the application's authenticated backup action. Do not copy an active SQLite database as a live backup.
- Review generated session logs after a failed startup. Do not publish them.

## Uploading source to GitHub

The repository at `https://github.com/tonexhow/ASFWMS` should receive **source only**. From the project root:

```powershell
git init
git branch -M main
git remote add origin https://github.com/tonexhow/ASFWMS.git
git add .
git status --short
git commit -m "Finalize ASFWMS SQLite scheduling and desktop release build"
git push -u origin main
```

Before committing, run `git status --short` to verify `.env`, `.db`, `seed.sql`, log files, and `dist/` are **absent** from staged files. If the repository already has a remote, use `git remote -v` and update it intentionally instead of adding a second origin. GitHub source does not include compiled binaries; run `compile.bat` to create a local private release.

## Known verification limitation

The release's full Go test/build commands require `modernc.org/sqlite v1.38.2`. This dependency was unavailable for download in the restricted development environment used to prepare the ZIP. The code's syntactic compile (using a temporary SQLite build tag), package-level Go checks, UI JS parsing, SQLite integrity checks, and lifecycle control tests passed. **Run normal `go test ./...` and `compile.bat` on your Windows machine before production deployment.**
