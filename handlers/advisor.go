package handlers

import (
	"fmt"
	"math"
	"strings"
	"time"

	"study-tracker/db"

	tele "gopkg.in/telebot.v3"
)

type AdvisorHandler struct {
	db *db.DB
}

func NewAdvisorHandler(database *db.DB) *AdvisorHandler {
	return &AdvisorHandler{db: database}
}

func (h *AdvisorHandler) Register(b *tele.Bot) {
	b.Handle("/students", h.listStudents)
	b.Handle("/monthly", h.monthlyReport)
	b.Handle("/promote", h.promoteToAdvisor)
}

// /students — list all registered students
func (h *AdvisorHandler) listStudents(c tele.Context) error {
	user, err := h.db.GetUser(c.Sender().ID)
	if err != nil || user == nil || user.Role != "advisor" {
		return c.Send("⛔ این دستور فقط برای مشاوران است.")
	}

	students, err := h.db.GetAllStudents()
	if err != nil || len(students) == 0 {
		return c.Send("هنوز هیچ دانش‌آموزی ثبت نشده.")
	}

	var sb strings.Builder
	sb.WriteString("👥 لیست دانش‌آموزان:\n\n")
	for i, s := range students {
		name := s.FullName
		if name == "" {
			name = "@" + s.Username
		}
		sb.WriteString(fmt.Sprintf("%d. %s (ID: %d)\n", i+1, name, s.TelegramID))
	}
	return c.Send(sb.String())
}

// /monthly <studentID> [YYYY-MM]
// Example: /monthly 123456789 2026-06
func (h *AdvisorHandler) monthlyReport(c tele.Context) error {
	user, err := h.db.GetUser(c.Sender().ID)
	if err != nil || user == nil || user.Role != "advisor" {
		return c.Send("⛔ این دستور فقط برای مشاوران است.")
	}

	args := c.Args() // telebot splits args for us
	if len(args) == 0 {
		return c.Send("استفاده: /monthly <studentID> [YYYY-MM]\nمثال: /monthly 123456789 2026-06")
	}

	var studentID int64
	if _, err := fmt.Sscan(args[0], &studentID); err != nil {
		return c.Send("⚠️ شناسه دانش‌آموز معتبر نیست.")
	}

	now := time.Now()
	year, month := now.Year(), int(now.Month())

	if len(args) >= 2 {
		t, err := time.Parse("2006-01", args[1])
		if err != nil {
			return c.Send("⚠️ فرمت تاریخ اشتباه است. از YYYY-MM استفاده کن.")
		}
		year, month = t.Year(), int(t.Month())
	}

	student, err := h.db.GetUser(studentID)
	if err != nil || student == nil {
		return c.Send("دانش‌آموزی با این شناسه پیدا نشد.")
	}

	reports, err := h.db.GetMonthlyReports(studentID, year, month)
	if err != nil {
		return c.Send("خطا در دریافت گزارش‌ها.")
	}
	if len(reports) == 0 {
		return c.Send(fmt.Sprintf("هیچ گزارشی برای %s در %04d-%02d ثبت نشده.", student.FullName, year, month))
	}

	// aggregate
	var totalHours float64
	var totalTests int
	for _, r := range reports {
		totalHours += r.StudyHours
		totalTests += r.TestCount
	}
	avgHours := totalHours / float64(len(reports))
	avgTests := float64(totalTests) / float64(len(reports))

	name := student.FullName
	if name == "" {
		name = "@" + student.Username
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 گزارش ماهانه — %s\n", name))
	sb.WriteString(fmt.Sprintf("📅 %04d-%02d\n\n", year, month))
	sb.WriteString(fmt.Sprintf("📆 روزهای ثبت‌شده: %d\n", len(reports)))
	sb.WriteString(fmt.Sprintf("📚 کل ساعت مطالعه: %.1f\n", totalHours))
	sb.WriteString(fmt.Sprintf("📝 کل تست‌ها: %d\n", totalTests))
	sb.WriteString(fmt.Sprintf("⌀ میانگین مطالعه روزانه: %.1f ساعت\n", math.Round(avgHours*10)/10))
	sb.WriteString(fmt.Sprintf("⌀ میانگین تست روزانه: %.1f\n\n", math.Round(avgTests*10)/10))

	sb.WriteString("📋 جزئیات روزانه:\n")
	for _, r := range reports {
		day := r.ReportedAt.Format("01/02")
		note := ""
		if r.Notes != "" {
			note = " — " + firstN(r.Notes, 40)
		}
		sb.WriteString(fmt.Sprintf("  %s: %.1fh | %d تست%s\n", day, r.StudyHours, r.TestCount, note))
	}

	return c.Send(sb.String())
}

// /promote <userID> — promote a user to advisor role
// Only existing advisors can promote others. First advisor must be set via DB or /start flow.
func (h *AdvisorHandler) promoteToAdvisor(c tele.Context) error {
	user, err := h.db.GetUser(c.Sender().ID)
	if err != nil || user == nil || user.Role != "advisor" {
		return c.Send("⛔ فقط مشاور می‌تواند کاربر دیگری را ارتقا دهد.")
	}

	args := c.Args()
	if len(args) == 0 {
		return c.Send("استفاده: /promote <userID>")
	}

	var targetID int64
	if _, err := fmt.Sscan(args[0], &targetID); err != nil {
		return c.Send("⚠️ شناسه معتبر نیست.")
	}

	target, err := h.db.GetUser(targetID)
	if err != nil || target == nil {
		return c.Send("کاربری با این شناسه پیدا نشد. کاربر باید ابتدا با ربات تعامل داشته باشد.")
	}

	if err := h.db.SetRole(targetID, "advisor"); err != nil {
		return c.Send("خطا در ارتقا.")
	}

	name := target.FullName
	if name == "" {
		name = "@" + target.Username
	}
	return c.Send(fmt.Sprintf("✅ %s به عنوان مشاور ثبت شد.", name))
}
