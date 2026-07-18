# Agent Guide — Study Tracker

## Project Overview

Study Tracker is a **Telegram bot** written in **Go** that helps students log daily study activity and lets advisors monitor their progress. The bot is built on [Telebot v3](https://gopkg.in/telebot.v3) with a local **SQLite** database.

All user-facing text is in **Persian (Farsi)**. Dates in advisor reports are shown in the **Jalali (Solar Hijri)** calendar via `github.com/yaa110/go-persian-calendar`.

---

## Tech Stack

| Layer       | Technology                             |
|-------------|----------------------------------------|
| Language    | Go 1.25                                |
| Bot API     | `gopkg.in/telebot.v3 v3.3.8`          |
| Database    | SQLite via `github.com/mattn/go-sqlite3 v1.14.42` |
| Calendar    | `github.com/yaa110/go-persian-calendar v1.3.0` |
| Build tool  | [Task](https://taskfile.dev) (`Taskfile.yml`) |

---

## Project Structure

```
study-tracker/
├── cmd/
│   ├── main.go          # Entry point: bot setup, env config, route registration
│   └── seedmock/        # CLI tool to seed mock students and reports
├── config/
│   └── config.go        # Loads BOT_TOKEN from environment (config.Load())
├── db/
│   ├── db.go            # All DB operations + auto-migration on startup
│   └── models.go        # User and DailyReport structs
├── handlers/
│   ├── student.go       # Student flows: /report, /report_edit, cancel, back
│   └── advisor.go       # Advisor flows: /students, /student, /monthly, /promote
├── Taskfile.yml         # Task runner (build, seed-mock)
├── data.db              # SQLite database (auto-created on first run)
└── go.mod / go.sum
```

---

## Environment Variables

| Variable         | Required | Description                                    |
|------------------|----------|------------------------------------------------|
| `BOT_TOKEN`      | Yes      | Telegram Bot API token                         |
| `FIRST_ADVISOR_ID` | No     | Telegram numeric ID to bootstrap as first advisor on startup |

---

## Running the Project

```bash
# Run directly
go run ./cmd/main.go

# Build for Linux
task build
# or manually:
GOOS=linux GOARCH=amd64 go build -o .build/bot cmd/main.go

# Seed mock data into data.db without Telegram
task seed-mock
# or manually:
go run ./cmd/seedmock -db ./data.db
```

---

## Data Layer (`db/`)

### Models (`db/models.go`)

```go
type User struct {
    TelegramID int64
    Username   string
    FullName   string
    Role       string    // "student" | "advisor"
    CreatedAt  time.Time
}

type DailyReport struct {
    ID         int64
    StudentID  int64
    StudyHours float64
    TestCount  int
    Notes      string
    ReportedAt time.Time
}
```

### Key DB Methods (`db/db.go`)

| Method | Description |
|--------|-------------|
| `UpsertUser(User)` | Insert or update user; never downgrades role |
| `GetUser(id)` | Fetch user by Telegram ID; returns `nil, nil` if not found |
| `SetRole(id, role)` | Change a user's role |
| `GetAllStudents()` | All users with role `"student"` |
| `GetStudentsPage(limit, offset)` | Paginated, name-sorted student list |
| `CountStudents()` | Total student count |
| `GetAllAdvisors()` | All users with role `"advisor"` |
| `SaveReport(DailyReport)` | Insert a new daily report |
| `UpdateReport(DailyReport)` | Update an existing report by ID and student ID |
| `GetTodayReport(studentID)` | Today's report for a student; `nil` if not submitted |
| `GetMonthlyReports(studentID, year, month)` | All reports for a given month |
| `GetStudentReportsPage(studentID, limit, offset)` | Paginated report history |
| `CountReportsByStudent(studentID)` | Total report count for a student |
| `SeedMockData()` | Insert 12 mock students with ~6–11 days of reports |

Schema is auto-migrated on every startup via `db.New(path)` — no manual migration step needed.

---

## Handlers

### Student Handler (`handlers/student.go`)

Manages a **multi-step conversational state machine** for report submission and editing. State is stored in an in-memory `map[int64]*studentState` protected by a `sync.Mutex`.

**State steps:**
1. `awaiting_hours` — ask for study hours (float, 0–24)
2. `awaiting_tests` — ask for test count (non-negative int)
3. `awaiting_notes` — ask for optional notes (or skip/keep)

**Key behaviors:**
- Auto-registers users on `/report` (calls `UpsertUser`)
- Blocks duplicate reports on the same day (shows existing report instead)
- `/report_edit` loads today's existing report into state with `IsEditing: true`
- After saving, notifies all advisors asynchronously via a goroutine
- `/cancel` clears state; `⬅️ بازگشت` steps back one stage

**Registered routes:**
- `/report`, `📝 ثبت گزارش` → `startReport`
- `/report_edit`, `✏️ ویرایش گزارش امروز` → `startEditReport`
- `/cancel` → `cancelReport`
- `tele.OnText` → `handleText` (drives the state machine)

### Advisor Handler (`handlers/advisor.go`)

Stateless. Uses **inline keyboard callbacks** with encoded payloads for pagination.

**Registered routes:**
- `/students`, `👥 دانش‌آموزها` → paginated student list
- `/student <studentID> [page]` → view a student's report history
- `/monthly <studentID> [YYYY-MM]` → generate a monthly summary
- `/promote <userID>` → promote a user to advisor role
- `📘 راهنمای مشاور` → show advisor command help
- Inline callbacks for page navigation and back button

**Access control:** All advisor routes call `requireAdvisor(c)` which checks the sender's role in the DB and returns an error message if unauthorized.

**Pagination constants:**
- `studentsPageSize = 5` students per page
- `reportsPageSize = 5` reports per page

---

## Entry Point (`cmd/main.go`)

- Loads `BOT_TOKEN` from env (fatal if missing)
- Opens SQLite DB via `db.New("./data.db")`
- Bootstraps `FIRST_ADVISOR_ID` if set (calls `UpsertUser` + `SetRole`)
- Creates `StudentHandler` and `AdvisorHandler`, calls `.Register(bot)` on both
- Registers `/start`, `/menu`, `/id`, and `🆔 آیدی من` directly in `main`
- Uses `tele.LongPoller` with 10s timeout

---

## Conventions & Patterns

- **Persian UI**: All messages, button labels, and error text are in Farsi.
- **Jalali dates**: Use `formatJalaliDate` / `formatJalaliDateTime` helpers in `advisor.go` (via `go-persian-calendar`).
- **Role check pattern**: Handlers call `requireAdvisor(c)` at the top; student handlers implicitly allow all users.
- **Nil-safe DB returns**: `GetUser` returns `(nil, nil)` when the user doesn't exist — always check for `nil` before using the result.
- **State machine thread safety**: Always use `getState`, `setState`, `clearState` helpers (which hold the mutex) — never access `h.states` directly.
- **Advisor notifications**: Always sent asynchronously with `go h.notifyAdvisors(...)` to avoid blocking the bot handler.
- **`firstN(s, n)`**: Truncates a string to `n` runes for safe message display. Returns `"—"` for empty strings.

---

## Common Tasks for an Agent

### Add a new student command
1. Add a handler method to `StudentHandler` in `handlers/student.go`
2. Register it in `StudentHandler.Register(b)` with `b.Handle(...)`
3. Optionally add a button to `MainMenu()`

### Add a new advisor command
1. Add a handler method to `AdvisorHandler` in `handlers/advisor.go`
2. Register it in `AdvisorHandler.Register(b)`
3. Call `requireAdvisor(c)` at the top of the handler
4. Optionally add a button to `MainMenu()`

### Add a new DB query
1. Add the method to `db/db.go` on the `*DB` receiver
2. If a new table is needed, add the `CREATE TABLE IF NOT EXISTS` statement to the `migrate()` method

### Extend the student state machine
- Add a new `Step` string constant and handle it in the `switch state.Step` block inside `handleText`
- Update `goBack` to step back from the new stage
- Add a matching prompt menu method

---

## Known Limitations / Future Work

- State is in-memory only — lost on restart; users mid-flow must start over.
- `config/config.go` exists but is not used by `cmd/main.go` (env vars are read directly).
- No weekly stats, leaderboard, CSV/PDF export, or reminder notifications yet.
- The `internal/` directory is currently empty.
