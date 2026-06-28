package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"study-tracker/db"
	"study-tracker/handlers"

	tele "gopkg.in/telebot.v3"
)

func main() {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	// optional: first advisor ID bootstrapped from env
	firstAdvisorID := os.Getenv("FIRST_ADVISOR_ID")

	database := db.New("./data.db")

	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal("failed to start bot:", err)
	}

	// bootstrap first advisor if provided
	if firstAdvisorID != "" {
		var id int64
		if _, err := fmt.Sscan(firstAdvisorID, &id); err == nil {
			_ = database.UpsertUser(db.User{TelegramID: id, Role: "advisor"})
			_ = database.SetRole(id, "advisor")
			log.Printf("bootstrapped advisor ID: %d\n", id)
		}
	}

	studentH := handlers.NewStudentHandler(database, bot)
	advisorH := handlers.NewAdvisorHandler(database)

	// Register all handler routes
	studentH.Register(bot)
	advisorH.Register(bot)

	// /start
	bot.Handle("/start", func(c tele.Context) error {
		user, err := database.GetUser(c.Sender().ID)
		if err != nil {
			return c.Send("خطا در ارتباط با پایگاه داده.")
		}

		if user == nil {
			if err := database.UpsertUser(db.User{
				TelegramID: c.Sender().ID,
				Username:   c.Sender().Username,
				FullName:   c.Sender().FirstName + " " + c.Sender().LastName,
				Role:       "student",
			}); err != nil {
				return c.Send("خطا در ثبت کاربر.")
			}

			return c.Send(
				"👋 خوش اومدی!\n\n" +
					"از دستور /report برای ثبت گزارش روزانه استفاده کن.",
			)
		}

		if user.Role == "advisor" {
			if handled, err := advisorH.HandleStudentPayload(c); handled || err != nil {
				return err
			}

			return c.Send(
				"👋 خوش برگشتی مشاور!\n\n" +
					"دستورات:\n" +
					"/students\n" +
					"/monthly\n" +
					"/promote",
			)
		}

		return c.Send(
			"👋 خوش برگشتی!\n\n" +
				"برای ثبت گزارش امروز از /report استفاده کن.",
		)
	})

	bot.Handle("/id", func(c tele.Context) error {
		return c.Send(
			"آیدی عددی شما 👇\n\n" +
				fmt.Sprintf("%d", c.Sender().ID),
		)
	})

	bot.Start()
}
