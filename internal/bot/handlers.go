package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready")
	b.registerCommands()
	b.updateBotStatus()
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.hasAdminOrModPermissions(s, i) {
		b.respondToInteraction(s, i, "You do not have permission to use this command.")
		return
	}

	switch i.ApplicationCommandData().Name {
	case "add":
		b.handleAddCommand(s, i)
	case "remove":
		b.handleRemoveCommand(s, i)
	case "list":
		b.handleListCommand(s, i)
	}
}

func (b *Bot) handleAddCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	channel := options[1].ChannelValue(s)
	var mentionRole string
	if len(options) > 2 {
		if role := options[2].RoleValue(s, i.GuildID); role != nil {
			mentionRole = role.ID
		}
	}

	accountInfo, err := b.APIClient.GetAccountInfo(username)
	if err != nil {
		log.Printf("Error getting account info for %s: %v", username, err)
		b.respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}

	if accountInfo == nil || accountInfo.Avatar.Locations == nil || len(accountInfo.Avatar.Variants) == 0 || len(accountInfo.Avatar.Variants[0].Locations) == 0 {
		log.Printf("Invalid account info structure for %s", username)
		b.respondToInteraction(s, i, "Error: Invalid account info structure")
		return
	}

	avatarLocation := accountInfo.Avatar.Variants[0].Locations[0].Location

	// Check if the account is already being followed
	myAccount, err := b.APIClient.GetMyAccountInfo()
	//fmt.Printf("%v", myAccount)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}
	if myAccount.ID == "" {
		b.respondToInteraction(s, i, "Error: Unable to retrieve account information")
		return
	}

	following, err := b.APIClient.GetFollowing(myAccount.ID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}

	isFollowing := false
	for _, f := range following {
		if f.AccountID == accountInfo.ID {
			isFollowing = true
			break
		}
	}

	if !isFollowing {
		err = b.APIClient.FollowAccount(accountInfo.ID)
		if err != nil {
			b.respondToInteraction(s, i, fmt.Sprintf("Error following account: %v", err))
			return
		}
	}

	// Store the monitored user in the database
	_, err = b.DB.Exec(`
        INSERT OR REPLACE INTO monitored_users 
        (guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at) 
        VALUES (?, ?, ?, ?, '', 0, ?, ?, ?)
    `, i.GuildID, accountInfo.ID, username, channel.ID, mentionRole, avatarLocation, time.Now().Unix())
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error storing user: %v", err))
		return
	}

	b.respondToInteraction(s, i, fmt.Sprintf("Added %s to monitoring list", username))
}

func (b *Bot) handleRemoveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	username := i.ApplicationCommandData().Options[0].StringValue()

	// Remove the monitored user from the database
	result, err := b.DB.Exec(`
        DELETE FROM monitored_users 
        WHERE guild_id = ? AND username = ?
    `, i.GuildID, username)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error removing user: %v", err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		b.respondToInteraction(s, i, fmt.Sprintf("User %s was not found in the monitoring list", username))
	} else {
		b.respondToInteraction(s, i, fmt.Sprintf("Removed %s from monitoring list", username))
	}
}

func (b *Bot) respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

func (b *Bot) handleListCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Fetch monitored users for the current guild
	rows, err := b.DB.Query(`
		SELECT username, notification_channel, mention_role 
		FROM monitored_users 
		WHERE guild_id = ?
	`, i.GuildID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error fetching monitored users: %v", err))
		return
	}
	defer rows.Close()

	var monitoredUsers []string
	for rows.Next() {
		var username, channelID, roleID string
		err := rows.Scan(&username, &channelID, &roleID)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		channelInfo := fmt.Sprintf("<#%s>", channelID)

		roleInfo := "No role"
		if roleID != "" {
			role, err := s.State.Role(i.GuildID, roleID)
			if err != nil {
				log.Printf("Error fetching role: %v", err)
			} else {
				roleInfo = role.Name
			}
		}

		userInfo := fmt.Sprintf("- %s (Channel: %s, Role: %s)", username, channelInfo, roleInfo)
		monitoredUsers = append(monitoredUsers, userInfo)
	}

	if len(monitoredUsers) == 0 {
		b.respondToInteraction(s, i, "No models are currently being monitored.")
		return
	}

	response := "Monitored models:\n" + strings.Join(monitoredUsers, "\n")
	b.respondToInteraction(s, i, response)
}

func (b *Bot) hasAdminOrModPermissions(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	// Check if the user is the server owner
	guild, err := s.Guild(i.GuildID)
	if err == nil && guild.OwnerID == i.Member.User.ID {
		return true
	}

	// Check for administrator permission
	if i.Member.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true
	}

	// Check for manage server permission (typically given to moderators)
	if i.Member.Permissions&discordgo.PermissionManageServer == discordgo.PermissionManageServer {
		return true
	}

	return false
}
