package config

import (
	"log"
	"os"
)

type Config struct {
	BotToken   string
	AdvisorIDs []uint64
}

func Load() *Config {
	
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN env var is required")
	}

	return &Config{
		BotToken: token,
	}

}
