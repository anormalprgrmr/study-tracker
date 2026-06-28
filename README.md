# 📚 Study Tracker Telegram Bot

A Telegram bot built with **Go** and **Telebot v3** that helps students submit daily study reports while allowing advisors to monitor progress and generate monthly reports.

## Features

### 👨‍🎓 Student

* Register automatically on first interaction
* Submit daily study reports
* Record:

  * Study hours
  * Number of practice tests
  * Daily notes
* Prevent duplicate reports on the same day
* Advisors receive instant notifications when a report is submitted

### 👨‍🏫 Advisor

* List all registered students
* View monthly reports for any student
* See:

  * Total study hours
  * Total tests completed
  * Daily averages
  * Daily report history
* Promote existing users to advisor

---

## Project Structure

```text
study-tracker/
│
├── db/
│   ├── db.go
│   ├── models.go
│   └── ...
│
├── handlers/
│   ├── student.go
│   └── advisor.go
│
├── main.go
├── go.mod
└── README.md
```

---

## Commands

### Student Commands

| Command   | Description                          |
| --------- | ------------------------------------ |
| `/start`  | Register and display welcome message |
| `/report` | Submit today's study report          |

### Advisor Commands

| Command                          | Description                  |
| -------------------------------- | ---------------------------- |
| `/students`                      | List all registered students |
| `/monthly <studentID> [YYYY-MM]` | Generate monthly report      |
| `/mockdata`                      | Seed mock students/reports   |
| `/promote <userID>`              | Promote a user to advisor    |

Example:

```text
/monthly 123456789 2026-06
```

---

## Daily Report Flow

```
/report

↓
Study Hours?

↓

Number of Tests?

↓

Notes?

↓

Report Saved

↓

Advisor Notification
```

---

## Environment Variables

| Variable           | Description                                 |
| ------------------ | ------------------------------------------- |
| `BOT_TOKEN`        | Telegram Bot Token                          |
| `FIRST_ADVISOR_ID` | (Optional) Telegram ID of the first advisor |

Example:

```bash
export BOT_TOKEN=123456:ABCDEF...
export FIRST_ADVISOR_ID=123456789
```

---

## Running the Project

Clone the repository:

```bash
git clone https://github.com/yourusername/study-tracker.git
cd study-tracker
```

Install dependencies:

```bash
go mod tidy
```

Run the bot:

```bash
go run .
```

or

```bash
go run main.go
```

Seed mock data without opening Telegram:

```bash
go run ./cmd/seedmock -db ./data.db
```

Or with Task:

```bash
task seed-mock
```

---

## Database

The project uses a local SQLite database.

The database file (`data.db`) is created automatically on first run.

---

## Built With

* Go
* Telebot v3
* SQLite

---

## Future Improvements

* Weekly statistics
* Student dashboard
* Report editing
* Leaderboard
* CSV/PDF report export
* Reminder notifications
* Admin panel
* Inline keyboards
* Pagination for student lists

---

## License

This project is licensed under the MIT License.

---

## Author

Developed as a simple Telegram-based study tracking system for students and advisors.
