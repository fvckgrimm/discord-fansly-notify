package bot

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/internal/config"
)

var tokenRegex = regexp.MustCompile(`[A-Za-z0-9]{30,}`)

func (b *Bot) ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready")
	b.registerCommands()
	b.updateBotStatus()
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.hasAdminOrModPermissions(s, i) {
		b.respondToInteraction(s, i, "You do not have permission to use this command.", false)
		return
	}

	switch i.ApplicationCommandData().Name {
	case "add":
		b.handleAddCommand(s, i)
	case "remove":
		b.handleRemoveCommand(s, i)
	case "list":
		b.handleListCommand(s, i)
	case "setliveimage":
		b.handleSetLiveImageCommand(s, i)
	case "toggle":
		b.handleToggleCommand(s, i)
	case "setchannel":
		b.handleSetChannelCommand(s, i)
	case "setpostmention":
		b.handleSetPostMentionCommand(s, i)
	case "setlivemention":
		b.handleSetLiveMentionCommand(s, i)
	}
}

func (b *Bot) handleAddCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()

	if tokenRegex.MatchString(username) {
		b.respondToInteraction(s, i, "Error: Username appears to contain a token. Please provide a valid username.", true)
		return
	}

	channel := options[1].ChannelValue(s)
	var mentionRole string
	if len(options) > 2 {
		if role := options[2].RoleValue(s, i.GuildID); role != nil {
			mentionRole = role.ID
		}
	}

	// Log What Creators are being monitored to better handle potential issues
	logMessage := fmt.Sprintf("`[ %s ]` %s:\n`Requested Creator Name:` **%s**\n----------",
		time.Now().Format("2006-01-02 15:04:05"),
		i.Member.User.Username,
		username,
	)

	_, err := s.ChannelMessageSend(config.LogChannelID, logMessage)
	if err != nil {
		log.Printf("Failed to send log message: %v", err)
	}

	accountInfo, err := b.APIClient.GetAccountInfo(username)
	if err != nil {
		log.Printf("Error getting account info for %s: %v", username, err)
		b.respondToInteraction(s, i, fmt.Sprintf("Error: %v", err), false)
		return
	}

	if accountInfo == nil /*|| accountInfo.Avatar.Locations == nil || len(accountInfo.Avatar.Variants) == 0 || len(accountInfo.Avatar.Variants[0].Locations) == 0*/ {
		log.Printf("Invalid account info structure for %s", username)
		b.respondToInteraction(s, i, "Error: Invalid account info structure", false)
		return
	}

	//avatarLocation := accountInfo.Avatar.Variants[0].Locations[0].Location
	// Improve getting avatar location
	var avatarLocation string
	if len(accountInfo.Avatar.Variants) > 0 && len(accountInfo.Avatar.Variants[0].Locations) > 0 {
		avatarLocation = accountInfo.Avatar.Variants[0].Locations[0].Location
	} else {
		log.Printf("Warning: No avatar found for user %s", username)
		avatarLocation = ""
	}

	// Check if timeline is accessible
	timelinePosts, timelineErr := b.APIClient.GetTimelinePost(accountInfo.ID)
	timelineAccessible := timelineErr == nil && len(timelinePosts) >= 0

	// Try to follow if timeline isn't accessible or required
	if !timelineAccessible {
		myAccount, err := b.APIClient.GetMyAccountInfo()
		if err == nil && myAccount.ID != "" {
			following, err := b.APIClient.GetFollowing(myAccount.ID)
			if err == nil {
				isFollowing := false
				for _, f := range following {
					if f.AccountID == accountInfo.ID {
						isFollowing = true
						break
					}
				}

				if !isFollowing {
					followErr := b.APIClient.FollowAccount(accountInfo.ID)
					if followErr != nil {
						log.Printf("Note: Could not follow %s: %v", username, followErr)
						//continue since we'll try to monitor
					}
				}
			}
		}

		timelinePosts, timelineErr = b.APIClient.GetTimelinePost(accountInfo.ID)
		timelineAccessible = timelineErr == nil
	}

	if !timelineAccessible {
		b.respondToInteraction(s, i, fmt.Sprintf("Cannot monitor %s: Timeline not accessible", username), false)
		return
	}

	// Store the monitored user in the database
	err = b.retryDbOperation(func() error {
		_, err = b.DB.Exec(`
			INSERT OR REPLACE INTO monitored_users 
			(guild_id, user_id, username, notification_channel, post_notification_channel, live_notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at, live_image_url, posts_enabled, live_enabled, live_mention_role, post_mention_role) 
			VALUES (?, ?, ?, ?, ?, ?, '', 0, ?, ?, ?, ?, 1, 1, ?, ?)
		`, i.GuildID, accountInfo.ID, username, channel.ID, channel.ID, channel.ID, mentionRole, avatarLocation, time.Now().Unix(), "", mentionRole, mentionRole)
		return err
	})

	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error storing user: %v", err), false)
		return
	}

	b.respondToInteraction(s, i, fmt.Sprintf("Added %s to monitoring list", username), false)
}

func (b *Bot) handleRemoveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	username := i.ApplicationCommandData().Options[0].StringValue()

	// Remove the monitored user from the database
	var rowsAffected int64
	err := b.retryDbOperation(func() error {
		result, err := b.DB.Exec(`
            DELETE FROM monitored_users 
            WHERE guild_id = ? AND username = ?
        `, i.GuildID, username)
		if err != nil {
			return err
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	})

	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error removing user: %v", err), false)
		return
	}

	if rowsAffected == 0 {
		b.respondToInteraction(s, i, fmt.Sprintf("User %s was not found in the monitoring list", username), false)
	} else {
		b.respondToInteraction(s, i, fmt.Sprintf("Removed %s from monitoring list", username), false)
	}
}

func (b *Bot) respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})
}

func (b *Bot) handleListCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Fetch monitored users for the current guild
	rows, err := b.DB.Query(`
		SELECT username, notification_channel, posts_enabled, live_enabled, live_mention_role, post_mention_role
		FROM monitored_users 
		WHERE guild_id = ?
	`, i.GuildID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error fetching monitored users: %v", err), false)
		return
	}
	defer rows.Close()

	var monitoredUsers []string
	for rows.Next() {
		var (
			username, channelID, postMentionRole, liveMentionRole string
			postsEnabled, liveEnabled                             bool
		)
		err := rows.Scan(&username, &channelID, &postsEnabled, &liveEnabled, &liveMentionRole, &postMentionRole)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		channelInfo := fmt.Sprintf("<#%s>", channelID)

		roleInfoPost := getRoleName(s, i.GuildID, postMentionRole)
		roleInfoLive := getRoleName(s, i.GuildID, liveMentionRole)

		// Create status indicators
		postStatus := "✅"
		if !postsEnabled {
			postStatus = "❌"
		}
		liveStatus := "✅"
		if !liveEnabled {
			liveStatus = "❌"
		}

		userInfo := fmt.Sprintf("- %s\n  • Channel: %s\n  • Live Role: %s\n • Post Role: %s\n  • Posts: %s\n  • Live: %s",
			username,
			channelInfo,
			roleInfoPost,
			roleInfoLive,
			postStatus,
			liveStatus,
		)
		monitoredUsers = append(monitoredUsers, userInfo)
	}

	if len(monitoredUsers) == 0 {
		b.respondToInteraction(s, i, "No models are currently being monitored.", false)
		return
	}

	response := "Monitored models:\n" + strings.Join(monitoredUsers, "\n")
	b.respondToInteraction(s, i, response, false)
}

func (b *Bot) handleSetLiveImageCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Acknowledge the interaction immediately
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error acknowledging interaction: %v", err)
		return
	}

	if !b.hasAdminOrModPermissions(s, i) {
		b.editInteractionResponse(s, i, "You need administrator permissions to use this command.")
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()

	var imageURL string
	if len(i.ApplicationCommandData().Resolved.Attachments) > 0 {
		for _, attachment := range i.ApplicationCommandData().Resolved.Attachments {
			imageURL = attachment.URL
			//log.Println(imageURL)
			break
		}
	}

	if imageURL == "" {
		b.editInteractionResponse(s, i, "Please attach an image to set as the live image.")
		return
	}

	// Update the database with the new live image URL
	_, err = b.DB.Exec(`
        UPDATE monitored_users 
        SET live_image_url = ? 
        WHERE guild_id = ? AND username = ?
    `, imageURL, i.GuildID, username)

	if err != nil {
		log.Printf("Error updating live image URL: %v", err)
		b.editInteractionResponse(s, i, "An error occurred while setting the live image.")
		return
	}

	b.editInteractionResponse(s, i, fmt.Sprintf("Live image for %s has been set successfully.", username))
}

func (b *Bot) handleToggleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	notifiType := options[1].StringValue()
	enabled := options[2].BoolValue()

	var column string
	switch notifiType {
	case "posts":
		column = "posts_enabled"
	case "live":
		column = "live_enabled"
	default:
		b.respondToInteraction(s, i, "Invalid notification type", false)
		return
	}

	query := fmt.Sprintf(`
		UPDATE monitored_users
		SET %s = ?
		WHERE guild_id = ? AND username = ?
		`, column)

	result, err := b.DB.Exec(query, enabled, i.GuildID, username)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating settings: %v", err), false)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		b.respondToInteraction(s, i, fmt.Sprintf("User %s not found", username), false)
		return
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}

	b.respondToInteraction(s, i, fmt.Sprintf("%s notifications %s for %s", notifiType, status, username), false)
}

func (b *Bot) handleSetChannelCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	notifType := options[1].StringValue()
	channel := options[2].ChannelValue(s)

	var columnName string
	switch notifType {
	case "posts":
		columnName = "post_notification_channel"
	case "live":
		columnName = "live_notification_channel"
	default:
		b.respondToInteraction(s, i, "Invalid notification type", false)
		return
	}

	query := fmt.Sprintf(`
        UPDATE monitored_users 
        SET %s = ? 
        WHERE guild_id = ? AND username = ?
    `, columnName)

	result, err := b.DB.Exec(query, channel.ID, i.GuildID, username)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating channel: %v", err), false)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		b.respondToInteraction(s, i, fmt.Sprintf("User %s not found", username), false)
		return
	}

	b.respondToInteraction(s, i, fmt.Sprintf("Successfully set %s notification channel for %s to %s", notifType, username, channel.Mention()), false)
}

func (b *Bot) handleSetPostMentionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	var roleID string
	if len(options) > 1 {
		role := options[1].RoleValue(s, i.GuildID)
		if role != nil {
			roleID = role.ID
		}
	}

	result, err := b.DB.Exec(`
        UPDATE monitored_users 
        SET post_mention_role = ? 
        WHERE guild_id = ? AND username = ?
    `, roleID, i.GuildID, username)

	if handleUpdateResponse(b, s, i, err, result, "post mention role", username, roleID) {
		return
	}
}

func (b *Bot) handleSetLiveMentionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	var roleID string
	if len(options) > 1 {
		role := options[1].RoleValue(s, i.GuildID)
		if role != nil {
			roleID = role.ID
		}
	}

	result, err := b.DB.Exec(`
        UPDATE monitored_users 
        SET live_mention_role = ? 
        WHERE guild_id = ? AND username = ?
    `, roleID, i.GuildID, username)

	if handleUpdateResponse(b, s, i, err, result, "live mention role", username, roleID) {
		return
	}
}

func handleUpdateResponse(b *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, err error, result sql.Result, roleType, username, roleID string) bool {
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating %s: %v", roleType, err), false)
		return true
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		b.respondToInteraction(s, i, fmt.Sprintf("User %s not found", username), false)
		return true
	}

	message := fmt.Sprintf("%s for %s has been cleared.", roleType, username)
	if roleID != "" {
		message = fmt.Sprintf("%s for %s set to <@&%s>", roleType, username, roleID)
	}
	b.respondToInteraction(s, i, message, false)
	return false
}

func getRoleName(s *discordgo.Session, guildID, roleID string) string {
	if roleID == "" {
		return "No role"
	}
	role, err := s.State.Role(guildID, roleID)
	if err != nil {
		return "Unknown role"
	}
	return role.Name
}

// Add this new helper function
func (b *Bot) editInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		log.Printf("Error editing interaction response: %v", err)
	}
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
