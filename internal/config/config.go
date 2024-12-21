package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

var (
	DiscordToken string
	FanslyToken  string
	UserAgent    string
	AppID        string
	PublicKey    string
	LogChannelID string
)

func Load() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	DiscordToken = os.Getenv("DISCORD_TOKEN")
	FanslyToken = os.Getenv("FANSLY_TOKEN")
	UserAgent = os.Getenv("USER_AGENT")
	AppID = os.Getenv("APP_ID")
	PublicKey = os.Getenv("PUBLIC_KEY")
	LogChannelID = os.Getenv("LOG_CHANNEL_ID")

	if DiscordToken == "" || FanslyToken == "" || UserAgent == "" || AppID == "" || PublicKey == "" || LogChannelID == "" {
		log.Fatal("Missing required environment variables")
	}
}
