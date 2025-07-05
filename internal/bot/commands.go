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
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "page",
					Description: "Page number to display",
					Required:    false,
				},
			},
		},
		{
			Name:        "setliveimage",
			Description: "Set a custom live image for a model",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "The username of the model",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "image",
					Description: "The image to use for live notifications",
					Required:    true,
				},
			},
		},
		{
			Name:        "toggle",
			Description: "Toggle notifications for a model",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "Notification type to toggle",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Posts",
							Value: "posts",
						},
						{
							Name:  "Live",
							Value: "live",
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionBoolean,
					Name:        "enabled",
					Description: "Enable or disable notifications",
					Required:    true,
				},
			},
		},
		{
			Name:        "setchannel",
			Description: "Set notification channel for posts or live notifications",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "notification type",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Posts",
							Value: "posts",
						},
						{
							Name:  "Live",
							Value: "live",
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The notification channel",
					Required:    true,
				},
			},
		},
		{
			Name:        "setpostmention",
			Description: "Set role to mention for post notifications",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to mention (optional)",
					Required:    false,
				},
			},
		},
		{
			Name:        "setlivemention",
			Description: "Set role to mention for live notifications",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "Fansly username",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to mention (optional)",
					Required:    false,
				},
			},
		},
		// --- NEW BOT OWNER COMMANDS ---
		{
			Name:        "servers",
			Description: "[Owner Only] List all servers the bot is in.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "page",
					Description: "Page number to display",
					Required:    false,
				},
			},
		},
		{
			Name:        "leave",
			Description: "[Owner Only] Make the bot leave a specific server.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "server",
					Description: "The ID or Name of the server to leave.",
					Required:    true,
				},
			},
		},
	}

	_, err := b.Session.ApplicationCommandBulkOverwrite(b.Session.State.User.ID, "", commands)
	if err != nil {
		log.Printf("Error registering commands: %v", err)
	}
}
