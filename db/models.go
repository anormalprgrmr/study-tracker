package db

import "time"

type User struct {
	TelegramID int64
	Username   string
	FullName   string
	Role       string // "student" | "advisor"
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
