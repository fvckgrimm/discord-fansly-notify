package bot

import (
	"github.com/bwmarrin/discordgo"
	"log"
)

func (b *Bot) registerCommands() {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "add",
			Description: "Add a Fansly model to monitor",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "Notification channel",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "mention_role",
					Description: "Role to mention (optional)",
					Required:    false,
				},
			},
		},
		{
			Name:        "remove",
			Description: "Remove a Fansly model from monitoring",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
			},
		},
		{
			Name:        "list",
			Description: "List all monitored models",
		},
	}

	_, err := b.Session.ApplicationCommandBulkOverwrite(b.Session.State.User.ID, "", commands)
	if err != nil {
		log.Printf("Error registering commands: %v", err)
	}
}
