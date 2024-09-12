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

	if DiscordToken == "" || FanslyToken == "" || UserAgent == "" || AppID == "" || PublicKey == "" {
		log.Fatal("Missing required environment variables")
	}
}
