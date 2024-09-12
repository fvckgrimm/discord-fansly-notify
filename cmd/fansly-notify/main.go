package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fvckgrimm/discord-fansly-notify/internal/bot"
	"github.com/fvckgrimm/discord-fansly-notify/internal/config"
	"github.com/fvckgrimm/discord-fansly-notify/internal/database"
)

func main() {
	config.Load()
	database.Init()
	defer database.Close()

	bot, err := bot.New()
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	err = bot.Start()
	if err != nil {
		log.Fatalf("Error starting bot: %v", err)
	}

	// Wait for a SIGINT or SIGTERM signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc

	bot.Stop()
}
