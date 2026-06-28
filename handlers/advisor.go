package handlers

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"

	"study-tracker/db"

	ptime "github.com/yaa110/go-persian-calendar"
	tele "gopkg.in/telebot.v3"
)

type AdvisorHandler struct {
	db          *db.DB
	botUsername string

	studentBtn      tele.Btn
	studentsPageBtn tele.Btn
	reportPageBtn   tele.Btn
	backBtn         tele.Btn
}

func NewAdvisorHandler(database *db.DB) *AdvisorHandler {
	return &AdvisorHandler{
		db:              database,
		studentBtn:      tele.Btn{Unique: "advisor_student"},
		studentsPageBtn: tele.Btn{Unique: "advisor_students_page"},
		reportPageBtn:   tele.Btn{Unique: "advisor_reports_page"},
		backBtn:         tele.Btn{Unique: "advisor_back_students"},
	}
}

func (h *AdvisorHandler) Register(b *tele.Bot) {
	if b.Me != nil {
		h.botUsername = b.Me.Username
	}
	b.Handle("/students", h.listStudents)
	b.Handle("/student", h.openStudentByCommand)
	b.Handle("/monthly", h.monthlyReport)
	b.Handle("/promote", h.promoteToAdvisor)
	b.Handle(&h.studentBtn, h.openStudentReports)
	b.Handle(&h.studentsPageBtn, h.changeStudentsPage)
	b.Handle(&h.reportPageBtn, h.changeReportPage)
	b.Handle(&h.backBtn, h.backToStudents)
}

// /students — list all registered students
func (h *AdvisorHandler) listStudents(c tele.Context) error {
	if _, err := h.requireAdvisor(c); err != nil {
		return err
	}

	return h.renderStudentsPage(c, 0)
}

func (h *AdvisorHandler) renderStudentsPage(c tele.Context, page int) error {
	totalStudents, err := h.db.CountStudents()
	if err != nil {
		return c.Send("خطا در دریافت لیست دانش‌آموزها.")
	}
	if totalStudents == 0 {
		return c.Send("هنوز هیچ دانش‌آموزی ثبت نشده.")
	}

	totalPages := totalPages(totalStudents, studentsPageSize)
	page = clampPage(page, totalPages)

	students, err := h.db.GetStudentsPage(studentsPageSize, page*studentsPageSize)
	if err != nil {
		return c.Send("خطا در دریافت لیست دانش‌آموزها.")
	}

	text := h.buildStudentsPageText(students, page, totalPages, totalStudents)
	markup := h.buildStudentsPageMarkup(students, page, totalPages)
	return c.EditOrSend(text, markup, tele.ModeHTML)
}

func (h *AdvisorHandler) buildStudentsPageText(students []db.User, page, totalPages, totalStudents int) string {
	var sb strings.Builder
	sb.WriteString("👥 لیست دانش‌آموزها\n\n")
	sb.WriteString(fmt.Sprintf("📊 تعداد کل: %d\n", totalStudents))
	sb.WriteString(fmt.Sprintf("📄 صفحه %d از %d\n\n", page+1, totalPages))
	sb.WriteString("روی اسم هر دانش‌آموز بزن تا گزارش‌هایش را ببینی.\n\n")

	start := page*studentsPageSize + 1
	for i, student := range students {
		sb.WriteString(fmt.Sprintf("%d. %s\n", start+i, h.studentReportLink(student, page)))
	}
	return sb.String()
}

func (h *AdvisorHandler) buildStudentsPageMarkup(students []db.User, page, totalPages int) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}

	var navRow []tele.Btn
	if page > 0 {
		navRow = append(navRow, markup.Data("⬅️ قبلی", h.studentsPageBtn.Unique, strconv.Itoa(page-1)))
	}
	if page < totalPages-1 {
		navRow = append(navRow, markup.Data("بعدی ➡️", h.studentsPageBtn.Unique, strconv.Itoa(page+1)))
	}
	if len(navRow) > 0 {
		markup.Inline(markup.Row(navRow...))
	}

	return markup
}

func (h *AdvisorHandler) HandleStudentPayload(c tele.Context) (bool, error) {
	studentID, studentsPage, ok := parseStudentPayload(strings.TrimSpace(c.Data()))
	if !ok {
		return false, nil
	}
	return true, h.renderStudentReportsForAdvisor(c, studentID, 0, studentsPage)
}

func (h *AdvisorHandler) openStudentByCommand(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send("استفاده: /student <studentID> [studentsPage]")
	}

	var studentID int64
	if _, err := fmt.Sscan(args[0], &studentID); err != nil {
		return c.Send("⚠️ شناسه دانش‌آموز معتبر نیست.")
	}

	studentsPage := 0
	if len(args) >= 2 {
		page, err := strconv.Atoi(args[1])
		if err != nil {
			return c.Send("⚠️ شماره صفحه معتبر نیست.")
		}
		studentsPage = page
	}

	return h.renderStudentReportsForAdvisor(c, studentID, 0, studentsPage)
}

func (h *AdvisorHandler) openStudentReports(c tele.Context) error {
	defer h.respondCallback(c)

	studentID, studentsPage, ok := parseStudentSelection(c.Data())
	if !ok {
		return c.Send("اطلاعات دانش‌آموز نامعتبر است.")
	}

	return h.renderStudentReportsForAdvisor(c, studentID, 0, studentsPage)
}

func (h *AdvisorHandler) changeStudentsPage(c tele.Context) error {
	if _, err := h.requireAdvisor(c); err != nil {
		return err
	}
	defer h.respondCallback(c)

	page, err := strconv.Atoi(c.Data())
	if err != nil {
		return c.Send("شماره صفحه نامعتبر است.")
	}
	return h.renderStudentsPage(c, page)
}

func (h *AdvisorHandler) changeReportPage(c tele.Context) error {
	defer h.respondCallback(c)

	studentID, reportsPage, studentsPage, ok := parseReportPageData(c.Data())
	if !ok {
		return c.Send("اطلاعات صفحه گزارش نامعتبر است.")
	}

	return h.renderStudentReportsForAdvisor(c, studentID, reportsPage, studentsPage)
}

func (h *AdvisorHandler) backToStudents(c tele.Context) error {
	if _, err := h.requireAdvisor(c); err != nil {
		return err
	}
	defer h.respondCallback(c)

	page, err := strconv.Atoi(c.Data())
	if err != nil {
		page = 0
	}
	return h.renderStudentsPage(c, page)
}

func (h *AdvisorHandler) renderStudentReportsForAdvisor(c tele.Context, studentID int64, reportsPage, studentsPage int) error {
	if _, err := h.requireAdvisor(c); err != nil {
		return err
	}
	return h.renderStudentReportsPage(c, studentID, reportsPage, studentsPage)
}

func (h *AdvisorHandler) renderStudentReportsPage(c tele.Context, studentID int64, reportsPage, studentsPage int) error {
	student, err := h.db.GetUser(studentID)
	if err != nil || student == nil {
		return c.Send("دانش‌آموزی با این شناسه پیدا نشد.")
	}

	totalReports, err := h.db.CountReportsByStudent(studentID)
	if err != nil {
		return c.Send("خطا در دریافت گزارش‌ها.")
	}

	if totalReports == 0 {
		text := fmt.Sprintf("📘 گزارش‌های %s\n\nهنوز گزارشی برای این دانش‌آموز ثبت نشده.", studentDisplayName(*student))
		return c.EditOrSend(text, h.buildEmptyReportsMarkup(studentsPage))
	}

	totalPages := totalPages(totalReports, reportsPageSize)
	reportsPage = clampPage(reportsPage, totalPages)

	reports, err := h.db.GetStudentReportsPage(studentID, reportsPageSize, reportsPage*reportsPageSize)
	if err != nil {
		return c.Send("خطا در دریافت گزارش‌ها.")
	}

	text := h.buildStudentReportsText(*student, reports, reportsPage, totalPages, totalReports)
	markup := h.buildStudentReportsMarkup(studentID, reportsPage, totalPages, studentsPage)
	return c.EditOrSend(text, markup)
}

func (h *AdvisorHandler) buildStudentReportsText(student db.User, reports []db.DailyReport, page, totalPages, totalReports int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📘 گزارش‌های %s\n\n", studentDisplayName(student)))
	sb.WriteString(fmt.Sprintf("🆔 %d\n", student.TelegramID))
	sb.WriteString(fmt.Sprintf("📊 تعداد کل گزارش‌ها: %d\n", totalReports))
	sb.WriteString(fmt.Sprintf("📄 صفحه %d از %d\n\n", page+1, totalPages))

	for i, report := range reports {
		sb.WriteString(fmt.Sprintf("%d. %s\n", page*reportsPageSize+i+1, formatJalaliDateTime(report.ReportedAt)))
		sb.WriteString(fmt.Sprintf("📚 %.1f ساعت | 📝 %d تست\n", report.StudyHours, report.TestCount))
		sb.WriteString(fmt.Sprintf("💬 %s\n\n", firstN(report.Notes, 80)))
	}
	return strings.TrimSpace(sb.String())
}

func (h *AdvisorHandler) buildStudentReportsMarkup(studentID int64, page, totalPages, studentsPage int) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}

	var navRow []tele.Btn
	if page > 0 {
		navRow = append(navRow, markup.Data("⬅️ قبلی", h.reportPageBtn.Unique, strconv.FormatInt(studentID, 10), strconv.Itoa(page-1), strconv.Itoa(studentsPage)))
	}
	back := markup.Data("↩️ بازگشت به دانش‌آموزها", h.backBtn.Unique, strconv.Itoa(studentsPage))
	navRow = append(navRow, back)
	if page < totalPages-1 {
		navRow = append(navRow, markup.Data("بعدی ➡️", h.reportPageBtn.Unique, strconv.FormatInt(studentID, 10), strconv.Itoa(page+1), strconv.Itoa(studentsPage)))
	}

	markup.Inline(markup.Row(navRow...))
	return markup
}

func (h *AdvisorHandler) buildEmptyReportsMarkup(studentsPage int) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	back := markup.Data("↩️ بازگشت به دانش‌آموزها", h.backBtn.Unique, strconv.Itoa(studentsPage))
	markup.Inline(markup.Row(back))
	return markup
}

func (h *AdvisorHandler) requireAdvisor(c tele.Context) (*db.User, error) {
	user, err := h.db.GetUser(c.Sender().ID)
	if err != nil || user == nil || user.Role != "advisor" {
		return nil, c.Send("⛔ این دستور فقط برای مشاوران است.")
	}
	return user, nil
}

func (h *AdvisorHandler) respondCallback(c tele.Context) {
	if c.Callback() == nil {
		return
	}
	_ = c.Respond()
}

func parseStudentSelection(data string) (studentID int64, studentsPage int, ok bool) {
	parts := strings.Split(data, "|")
	if len(parts) != 2 {
		return 0, 0, false
	}
	studentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	studentsPage, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return studentID, studentsPage, true
}

func parseReportPageData(data string) (studentID int64, reportsPage int, studentsPage int, ok bool) {
	parts := strings.Split(data, "|")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}

	studentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	reportsPage, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	studentsPage, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, false
	}
	return studentID, reportsPage, studentsPage, true
}

func studentDisplayName(student db.User) string {
	name := strings.TrimSpace(student.FullName)
	if name != "" {
		return name
	}
	if student.Username != "" {
		return "@" + student.Username
	}
	return fmt.Sprintf("کاربر %d", student.TelegramID)
}

func (h *AdvisorHandler) studentReportLink(student db.User, studentsPage int) string {
	name := html.EscapeString(studentDisplayName(student))
	if h.botUsername == "" {
		return fmt.Sprintf("/student %d %d", student.TelegramID, studentsPage)
	}
	return fmt.Sprintf(
		"<a href='https://t.me/%s?start=%s'>%s</a>",
		h.botUsername,
		studentPayload(student.TelegramID, studentsPage),
		name,
	)
}

func studentPayload(studentID int64, studentsPage int) string {
	return fmt.Sprintf("student_%d_%d", studentID, studentsPage)
}

func parseStudentPayload(payload string) (studentID int64, studentsPage int, ok bool) {
	parts := strings.Split(payload, "_")
	if len(parts) != 3 || parts[0] != "student" {
		return 0, 0, false
	}
	studentID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	studentsPage, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, false
	}
	return studentID, studentsPage, true
}

func totalPages(total, pageSize int) int {
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func clampPage(page, totalPages int) int {
	if page < 0 {
		return 0
	}
	if page >= totalPages {
		return totalPages - 1
	}
	return page
}

const (
	studentsPageSize = 8
	reportsPageSize  = 5
)

func formatJalaliDateTime(t time.Time) string {
	pt := ptime.New(t)
	year, month, day := pt.Date()
	hour, minute, _ := pt.Clock()
	return fmt.Sprintf("%04d/%02d/%02d %02d:%02d", year, int(month), day, hour, minute)
}

func formatJalaliDate(t time.Time) string {
	pt := ptime.New(t)
	year, month, day := pt.Date()
	return fmt.Sprintf("%04d/%02d/%02d", year, int(month), day)
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
		day := formatJalaliDate(r.ReportedAt)
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
