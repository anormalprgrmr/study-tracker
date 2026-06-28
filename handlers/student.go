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
	ReportID   int64
	StudyHours float64
	TestCount  int
	Notes      string
	IsEditing  bool
}

type StudentHandler struct {
	db     *db.DB
	bot    *tele.Bot
	mu     sync.Mutex
	states map[int64]*studentState // keyed by telegram user ID

	reportMenuBtn tele.Btn
	editMenuBtn   tele.Btn
}

func NewStudentHandler(database *db.DB, bot *tele.Bot) *StudentHandler {
	return &StudentHandler{
		db:            database,
		bot:           bot,
		states:        make(map[int64]*studentState),
		reportMenuBtn: tele.Btn{Text: "📝 ثبت گزارش"},
		editMenuBtn:   tele.Btn{Text: "✏️ ویرایش گزارش امروز"},
	}
}

func (h *StudentHandler) Register(b *tele.Bot) {
	b.Handle("/report", h.startReport)
	b.Handle("/report_edit", h.startEditReport)
	b.Handle("/cancel", h.cancelReport)
	b.Handle(&h.reportMenuBtn, h.startReport)
	b.Handle(&h.editMenuBtn, h.startEditReport)
	b.Handle(tele.OnText, h.handleText)
}

func (h *StudentHandler) MainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
	menu.Reply(
		menu.Row(h.reportMenuBtn),
		menu.Row(h.editMenuBtn),
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
			"✅ امروز قبلاً گزارش دادی:\n📚 ساعت مطالعه: %.1f\n📝 تعداد تست: %d\n💬 یادداشت: %s\n\nاگر خواستی از «✏️ ویرایش گزارش امروز» استفاده کن.",
			existing.StudyHours, existing.TestCount, firstN(existing.Notes, 100),
		), h.MainMenu())
	}

	h.setState(userID, &studentState{Step: "awaiting_hours"})

	return c.Send(
		"📊 گزارش امروزت رو شروع می‌کنیم!\n\n"+
			"چند ساعت مطالعه داشتی؟\nمثلاً `3.5`",
		h.hoursPromptMenu(),
		tele.ModeMarkdown,
	)
}

func (h *StudentHandler) startEditReport(c tele.Context) error {
	userID := c.Sender().ID

	existing, err := h.db.GetTodayReport(userID)
	if err != nil {
		return c.Send("خطایی رخ داد، لطفاً دوباره تلاش کن.")
	}
	if existing == nil {
		return c.Send(
			"امروز هنوز گزارشی ثبت نکردی.\nاول از «📝 ثبت گزارش» استفاده کن.",
			h.MainMenu(),
		)
	}

	h.setState(userID, &studentState{
		Step:       "awaiting_hours",
		ReportID:   existing.ID,
		StudyHours: existing.StudyHours,
		TestCount:  existing.TestCount,
		Notes:      existing.Notes,
		IsEditing:  true,
	})

	return c.Send(
		fmt.Sprintf(
			"✏️ ویرایش گزارش امروز\n\n📚 ساعت فعلی: %.1f\n📝 تست فعلی: %d\n💬 یادداشت فعلی: %s\n\nعدد جدید ساعت مطالعه را بفرست.",
			existing.StudyHours,
			existing.TestCount,
			firstN(existing.Notes, 100),
		),
		h.hoursPromptMenu(),
	)
}

func (h *StudentHandler) handleText(c tele.Context) error {
	userID := c.Sender().ID

	state, ok := h.getState(userID)

	if !ok {
		// not in a flow, ignore or hint
		return nil
	}

	text := strings.TrimSpace(c.Text())

	switch text {
	case cancelReportText:
		return h.cancelReport(c)
	case backReportText:
		return h.goBack(c, state)
	}

	switch state.Step {
	case "awaiting_hours":
		hours, err := strconv.ParseFloat(text, 64)
		if err != nil || hours < 0 || hours > 24 {
			return c.Send(
				"⚠️ عدد معتبری وارد کن. مثلاً `2` یا `3.5`",
				h.hoursPromptMenu(),
				tele.ModeMarkdown,
			)
		}
		state.StudyHours = hours
		state.Step = "awaiting_tests"
		msg := fmt.Sprintf(
			"✅ ساعت مطالعه ثبت شد: %.1f ساعت\n\nچند تا تست زدی؟\nاگه تست نزدی `0` بفرست.",
			state.StudyHours,
		)
		if state.IsEditing {
			msg = fmt.Sprintf(
				"✅ ساعت مطالعه جدید ثبت شد: %.1f ساعت\n\nتعداد تست جدید را بفرست.\nمقدار قبلی: %d",
				state.StudyHours,
				state.TestCount,
			)
		}
		return c.Send(
			msg,
			h.testsPromptMenu(),
			tele.ModeMarkdown,
		)

	case "awaiting_tests":
		count, err := strconv.Atoi(text)
		if err != nil || count < 0 {
			return c.Send("⚠️ یه عدد صحیح و صفر یا مثبت وارد کن.", h.testsPromptMenu())
		}
		state.TestCount = count
		state.Step = "awaiting_notes"
		if state.IsEditing {
			return c.Send(
				fmt.Sprintf(
					"💬 یادداشت جدید را بفرست.\nیادداشت فعلی: %s\nاگر نمی‌خواهی تغییر کند، «بدون تغییر یادداشت» را بزن.",
					firstN(state.Notes, 100),
				),
				h.notesMenu(true),
			)
		}
		return c.Send(
			"💬 اگر خواستی یک یادداشت کوتاه بنویس.\nاگر یادداشتی نداری، دکمه «بدون یادداشت» را بزن.",
			h.notesMenu(false),
		)

	case "awaiting_notes":
		notes := text
		if state.IsEditing && notes == keepNotesText {
			notes = state.Notes
		}
		if notes == skipNotesText || notes == "-" {
			notes = ""
		}

		report := db.DailyReport{
			ID:         state.ReportID,
			StudentID:  userID,
			StudyHours: state.StudyHours,
			TestCount:  state.TestCount,
			Notes:      notes,
		}

		var saveErr error
		if state.IsEditing {
			saveErr = h.db.UpdateReport(report)
		} else {
			saveErr = h.db.SaveReport(report)
		}
		if saveErr != nil {
			return c.Send("❌ خطا در ذخیره گزارش، دوباره امتحان کن.", h.notesMenu(state.IsEditing))
		}

		h.clearState(userID)

		// notify advisors
		go h.notifyAdvisors(c.Sender(), report, state.IsEditing)

		if state.IsEditing {
			return c.Send(fmt.Sprintf(
				"✅ گزارش امروزت ویرایش شد!\n\n📚 مطالعه: %.1f ساعت\n📝 تست: %d عدد\n💬 یادداشت: %s",
				report.StudyHours, report.TestCount, firstN(notes, 100),
			), h.MainMenu())
		}
		return c.Send(fmt.Sprintf(
			"✅ گزارشت ثبت شد!\n\n📚 مطالعه: %.1f ساعت\n📝 تست: %d عدد\n💬 یادداشت: %s",
			report.StudyHours, report.TestCount, firstN(notes, 100),
		), h.MainMenu())
	}

	return nil
}

func (h *StudentHandler) cancelReport(c tele.Context) error {
	_, ok := h.getState(c.Sender().ID)
	if ok {
		h.clearState(c.Sender().ID)
		return c.Send("گزارش نیمه‌کاره لغو شد. هر وقت خواستی دوباره از منو شروع کن.", h.MainMenu())
	}
	return c.Send("الان گزارشی در حال ثبت نداری.", h.MainMenu())
}

func (h *StudentHandler) goBack(c tele.Context, state *studentState) error {
	switch state.Step {
	case "awaiting_tests":
		state.Step = "awaiting_hours"
		return c.Send(
			"⬅️ برگشتیم به مرحله ساعت مطالعه.\nچند ساعت مطالعه داشتی؟",
			h.hoursPromptMenu(),
		)
	case "awaiting_notes":
		state.Step = "awaiting_tests"
		return c.Send("⬅️ برگشتیم به مرحله تعداد تست.\nچند تا تست زدی؟", h.testsPromptMenu())
	default:
		return c.Send("الان در اولین مرحله هستی. برای خروج از فرایند، «لغو» را بزن.", h.hoursPromptMenu())
	}
}

func (h *StudentHandler) hoursPromptMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text(cancelReportText)),
	)
	return menu
}

func (h *StudentHandler) testsPromptMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text(backReportText), menu.Text(cancelReportText)),
	)
	return menu
}

func (h *StudentHandler) notesMenu(isEditing bool) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	if isEditing {
		menu.Reply(
			menu.Row(menu.Text(keepNotesText)),
			menu.Row(menu.Text(skipNotesText)),
			menu.Row(menu.Text(backReportText), menu.Text(cancelReportText)),
		)
		return menu
	}
	menu.Reply(
		menu.Row(menu.Text(skipNotesText)),
		menu.Row(menu.Text(backReportText), menu.Text(cancelReportText)),
	)
	return menu
}

func (h *StudentHandler) getState(userID int64) (*studentState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, ok := h.states[userID]
	return state, ok
}

func (h *StudentHandler) setState(userID int64, state *studentState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.states[userID] = state
}

func (h *StudentHandler) clearState(userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.states, userID)
}

func (h *StudentHandler) notifyAdvisors(sender *tele.User, r db.DailyReport, isEditing bool) {
	advisors, err := h.db.GetAllAdvisors()
	if err != nil {
		log.Println("failed to fetch advisors:", err)
		return
	}

	name := strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	if name == "" {
		name = "@" + sender.Username
	}

	title := "📬 گزارش جدید"
	if isEditing {
		title = "✏️ گزارش ویرایش شد"
	}
	msg := fmt.Sprintf(
		"%s از %s\n\n📚 ساعت مطالعه: %.1f\n📝 تعداد تست: %d\n💬 یادداشت: %s",
		title, name, r.StudyHours, r.TestCount, firstN(r.Notes, 200),
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

const (
	cancelReportText = "❌ لغو"
	backReportText   = "⬅️ بازگشت"
	skipNotesText    = "⏭ بدون یادداشت"
	keepNotesText    = "📝 بدون تغییر یادداشت"
)
