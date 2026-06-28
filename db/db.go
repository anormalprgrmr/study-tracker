package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(path string) *DB {
	conn, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		log.Fatal("failed to open db:", err)
	}
	d := &DB{conn: conn}
	d.migrate()
	return d
}

func (d *DB) migrate() {
	schema := `
    CREATE TABLE IF NOT EXISTS users (
        telegram_id INTEGER PRIMARY KEY,
        username    TEXT,
        full_name   TEXT,
        role        TEXT NOT NULL DEFAULT 'student',
        created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    CREATE TABLE IF NOT EXISTS daily_reports (
        id           INTEGER PRIMARY KEY AUTOINCREMENT,
        student_id   INTEGER NOT NULL,
        study_hours  REAL    NOT NULL,
        test_count   INTEGER NOT NULL,
        notes        TEXT,
        reported_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (student_id) REFERENCES users(telegram_id)
    );`
	if _, err := d.conn.Exec(schema); err != nil {
		log.Fatal("migration failed:", err)
	}
}

// UpsertUser registers or updates a user (does not change role on conflict).
func (d *DB) UpsertUser(u User) error {
	_, err := d.conn.Exec(`
        INSERT INTO users (telegram_id, username, full_name, role)
        VALUES (?, ?, ?, 'student')
        ON CONFLICT(telegram_id) DO UPDATE SET
            username  = excluded.username,
            full_name = excluded.full_name
    `, u.TelegramID, u.Username, u.FullName)
	return err
}

func (d *DB) GetUser(telegramID int64) (*User, error) {
	row := d.conn.QueryRow(
		`SELECT telegram_id, username, full_name, role, created_at FROM users WHERE telegram_id = ?`,
		telegramID,
	)
	var u User
	if err := row.Scan(&u.TelegramID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (d *DB) SetRole(telegramID int64, role string) error {
	_, err := d.conn.Exec(`UPDATE users SET role = ? WHERE telegram_id = ?`, role, telegramID)
	return err
}

func (d *DB) GetAllStudents() ([]User, error) {
	return d.getUsersByRole("student")
}

func (d *DB) CountStudents() (int, error) {
	row := d.conn.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'student'`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *DB) GetStudentsPage(limit, offset int) ([]User, error) {
	rows, err := d.conn.Query(
		`SELECT telegram_id, username, full_name, role, created_at
		 FROM users
		 WHERE role = 'student'
		 ORDER BY COALESCE(NULLIF(full_name, ''), NULLIF(username, ''), CAST(telegram_id AS TEXT)) COLLATE NOCASE ASC
		 LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (d *DB) GetAllAdvisors() ([]User, error) {
	return d.getUsersByRole("advisor")
}

func (d *DB) getUsersByRole(role string) ([]User, error) {
	rows, err := d.conn.Query(
		`SELECT telegram_id, username, full_name, role, created_at FROM users WHERE role = ?`, role,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FullName, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (d *DB) SaveReport(r DailyReport) error {
	_, err := d.conn.Exec(`
        INSERT INTO daily_reports (student_id, study_hours, test_count, notes, reported_at)
        VALUES (?, ?, ?, ?, ?)
    `, r.StudentID, r.StudyHours, r.TestCount, r.Notes, time.Now())
	return err
}

// GetTodayReport returns today's report for a student if it exists.
func (d *DB) GetTodayReport(studentID int64) (*DailyReport, error) {
	row := d.conn.QueryRow(`
        SELECT id, student_id, study_hours, test_count, notes, reported_at
        FROM daily_reports
        WHERE student_id = ? AND date(reported_at) = date('now')
        LIMIT 1
    `, studentID)
	var r DailyReport
	if err := row.Scan(&r.ID, &r.StudentID, &r.StudyHours, &r.TestCount, &r.Notes, &r.ReportedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetMonthlyReports returns all reports for a student in a given year/month.
func (d *DB) GetMonthlyReports(studentID int64, year, month int) ([]DailyReport, error) {
	rows, err := d.conn.Query(`
        SELECT id, student_id, study_hours, test_count, notes, reported_at
        FROM daily_reports
        WHERE student_id = ?
          AND strftime('%Y', reported_at) = ?
          AND strftime('%m', reported_at) = ?
        ORDER BY reported_at ASC
    `, studentID,
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []DailyReport
	for rows.Next() {
		var r DailyReport
		if err := rows.Scan(&r.ID, &r.StudentID, &r.StudyHours, &r.TestCount, &r.Notes, &r.ReportedAt); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (d *DB) CountReportsByStudent(studentID int64) (int, error) {
	row := d.conn.QueryRow(`SELECT COUNT(*) FROM daily_reports WHERE student_id = ?`, studentID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *DB) GetStudentReportsPage(studentID int64, limit, offset int) ([]DailyReport, error) {
	rows, err := d.conn.Query(`
        SELECT id, student_id, study_hours, test_count, notes, reported_at
        FROM daily_reports
        WHERE student_id = ?
        ORDER BY reported_at DESC
        LIMIT ? OFFSET ?
    `, studentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []DailyReport
	for rows.Next() {
		var r DailyReport
		if err := rows.Scan(&r.ID, &r.StudentID, &r.StudyHours, &r.TestCount, &r.Notes, &r.ReportedAt); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (d *DB) SeedMockData() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	mockStudents := []User{
		{TelegramID: 900001, Username: "sara_ahmadi", FullName: "Sara Ahmadi"},
		{TelegramID: 900002, Username: "amir_hosseini", FullName: "Amir Hosseini"},
		{TelegramID: 900003, Username: "nazanin_moradi", FullName: "Nazanin Moradi"},
		{TelegramID: 900004, Username: "ali_karimi", FullName: "Ali Karimi"},
		{TelegramID: 900005, Username: "parsa_jalili", FullName: "Parsa Jalili"},
		{TelegramID: 900006, Username: "fatemeh_rostami", FullName: "Fatemeh Rostami"},
		{TelegramID: 900007, Username: "mahdi_shiri", FullName: "Mahdi Shiri"},
		{TelegramID: 900008, Username: "yasaman_nikoo", FullName: "Yasaman Nikoo"},
		{TelegramID: 900009, Username: "armin_davari", FullName: "Armin Davari"},
		{TelegramID: 900010, Username: "zahra_motiei", FullName: "Zahra Motiei"},
		{TelegramID: 900011, Username: "erfan_taheri", FullName: "Erfan Taheri"},
		{TelegramID: 900012, Username: "melika_noori", FullName: "Melika Noori"},
	}

	userStmt, err := tx.Prepare(`
		INSERT INTO users (telegram_id, username, full_name, role)
		VALUES (?, ?, ?, 'student')
		ON CONFLICT(telegram_id) DO UPDATE SET
			username = excluded.username,
			full_name = excluded.full_name
	`)
	if err != nil {
		return err
	}
	defer userStmt.Close()

	reportStmt, err := tx.Prepare(`
		INSERT INTO daily_reports (student_id, study_hours, test_count, notes, reported_at)
		SELECT ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM daily_reports
			WHERE student_id = ? AND date(reported_at) = date(?)
		)
	`)
	if err != nil {
		return err
	}
	defer reportStmt.Close()

	notes := []string{
		"مرور زیست و حل تست زمان‌دار.",
		"جمع‌بندی ریاضی و بررسی غلط‌ها.",
		"مطالعه مفهومی شیمی و خلاصه‌نویسی.",
		"تمرین ادبیات و تحلیل آزمون.",
		"مرور فیزیک با تمرکز روی مباحث ضعف.",
	}

	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	baseDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -14)
	for i, student := range mockStudents {
		if _, err := userStmt.Exec(student.TelegramID, student.Username, student.FullName); err != nil {
			return err
		}

		days := 6 + (i % 6)
		for day := 0; day < days; day++ {
			reportTime := baseDate.AddDate(0, 0, day).Add(time.Duration((i+day)%5+16)*time.Hour + time.Duration((i*13+day*7)%60)*time.Minute)
			studyHours := 2.5 + float64((i+day)%5)*0.75
			testCount := 20 + ((i*7 + day*11) % 55)
			note := notes[(i+day)%len(notes)]

			if _, err := reportStmt.Exec(
				student.TelegramID,
				studyHours,
				testCount,
				note,
				reportTime,
				student.TelegramID,
				reportTime,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
