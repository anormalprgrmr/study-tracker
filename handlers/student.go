package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"study-tracker/db"

	tele "gopkg.in/telebot.v3"
)

type studentState struct {
	Step       string
	StudyHours float64
	TestCount  int
}

type StudentHandler struct {
	db     *db.DB
	bot    *tele.Bot
	mu     sync.Mutex
	states map[int64]*studentState // keyed by telegram user ID

	reportMenuBtn tele.Btn
}

func NewStudentHandler(database *db.DB, bot *tele.Bot) *StudentHandler {
	return &StudentHandler{
		db:            database,
		bot:           bot,
		states:        make(map[int64]*studentState),
		reportMenuBtn: tele.Btn{Text: "📝 ثبت گزارش"},
	}
}

func (h *StudentHandler) Register(b *tele.Bot) {
	b.Handle("/report", h.startReport)
	b.Handle(&h.reportMenuBtn, h.startReport)
	b.Handle(tele.OnText, h.handleText)
}

func (h *StudentHandler) MainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
	menu.Reply(
		menu.Row(h.reportMenuBtn),
	)
	return menu
}

func (h *StudentHandler) startReport(c tele.Context) error {
	userID := c.Sender().ID

	// auto-register user
	if err := h.db.UpsertUser(db.User{
		TelegramID: userID,
		Username:   c.Sender().Username,
		FullName:   strings.TrimSpace(c.Sender().FirstName + " " + c.Sender().LastName),
	}); err != nil {
		log.Println("upsert error:", err)
	}

	// check if already reported today
	existing, err := h.db.GetTodayReport(userID)
	if err != nil {
		return c.Send("خطایی رخ داد، لطفاً دوباره تلاش کن.")
	}
	if existing != nil {
		return c.Send(fmt.Sprintf(
			"✅ امروز قبلاً گزارش دادی:\n📚 ساعت مطالعه: %.1f\n📝 تعداد تست: %d\n\nاگه می‌خوای ویرایش کنی /report_edit رو بزن.",
			existing.StudyHours, existing.TestCount,
		))
	}

	h.mu.Lock()
	h.states[userID] = &studentState{Step: "awaiting_hours"}
	h.mu.Unlock()

	return c.Send("📊 گزارش امروزت رو شروع می‌کنیم!\n\nچند ساعت مطالعه داشتی؟ (مثلاً: 3.5)")
}

func (h *StudentHandler) handleText(c tele.Context) error {
	userID := c.Sender().ID

	h.mu.Lock()
	state, ok := h.states[userID]
	h.mu.Unlock()

	if !ok {
		// not in a flow, ignore or hint
		return nil
	}

	text := strings.TrimSpace(c.Text())

	switch state.Step {
	case "awaiting_hours":
		hours, err := strconv.ParseFloat(text, 64)
		if err != nil || hours < 0 || hours > 24 {
			return c.Send("⚠️ عدد معتبری وارد کن (مثلاً 2 یا 3.5)")
		}
		state.StudyHours = hours
		state.Step = "awaiting_tests"
		return c.Send("چند تا تست زدی؟ (اگه نزدی 0 بنویس)")

	case "awaiting_tests":
		count, err := strconv.Atoi(text)
		if err != nil || count < 0 {
			return c.Send("⚠️ یه عدد صحیح و مثبت وارد کن")
		}
		state.TestCount = count
		state.Step = "awaiting_notes"
		return c.Send("یه یادداشت کوتاه بنویس (اگه چیزی نداری بنویس «-»)")

	case "awaiting_notes":
		notes := text
		if notes == "-" {
			notes = ""
		}

		report := db.DailyReport{
			StudentID:  userID,
			StudyHours: state.StudyHours,
			TestCount:  state.TestCount,
			Notes:      notes,
		}

		if err := h.db.SaveReport(report); err != nil {
			return c.Send("❌ خطا در ذخیره گزارش، دوباره امتحان کن.")
		}

		h.mu.Lock()
		delete(h.states, userID)
		h.mu.Unlock()

		// notify advisors
		go h.notifyAdvisors(c.Sender(), report)

		return c.Send(fmt.Sprintf(
			"✅ گزارشت ثبت شد!\n\n📚 مطالعه: %.1f ساعت\n📝 تست: %d عدد\n💬 یادداشت: %s",
			report.StudyHours, report.TestCount, firstN(notes, 100),
		))
	}

	return nil
}

func (h *StudentHandler) notifyAdvisors(sender *tele.User, r db.DailyReport) {
	advisors, err := h.db.GetAllAdvisors()
	if err != nil {
		log.Println("failed to fetch advisors:", err)
		return
	}

	name := strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	if name == "" {
		name = "@" + sender.Username
	}

	msg := fmt.Sprintf(
		"📬 گزارش جدید از %s\n\n📚 ساعت مطالعه: %.1f\n📝 تعداد تست: %d\n💬 یادداشت: %s",
		name, r.StudyHours, r.TestCount, firstN(r.Notes, 200),
	)

	for _, advisor := range advisors {
		if _, err := h.bot.Send(tele.ChatID(advisor.TelegramID), msg); err != nil {
			log.Printf("failed to notify advisor %d: %v\n", advisor.TelegramID, err)
		}
	}
}

func firstN(s string, n int) string {
	if s == "" {
		return "—"
	}
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "..."
	}
	return s
}
