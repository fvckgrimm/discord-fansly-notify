package bot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/internal/config"
	"github.com/fvckgrimm/discord-fansly-notify/internal/database"
	"github.com/fvckgrimm/discord-fansly-notify/internal/models"
)

var (
	tokenRegex     = regexp.MustCompile(`[A-Za-z0-9]{40,}`)
	fanslyURLRegex = regexp.MustCompile(`(?:https?://)?(?:www\.)?(?:fans\.ly|fansly\.com)/([^/\s]+)(?:/.*)?`)
)

func (b *Bot) ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready")
	b.registerCommands()
	b.updateBotStatus()
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Handle different interaction types
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		// Only check permissions for application commands
		if !b.hasAdminOrModPermissions(s, i) {
			username := "User"
			if i.User != nil {
				username = i.User.Username
			} else if i.Member != nil && i.Member.User != nil {
				username = i.Member.User.Username
			}
			b.respondToInteraction(s, i, "You do not have permission to use this command.", false)
			log.Printf("Permission denied for user %s", username)
			return
		}

		// Handle application commands
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

	case discordgo.InteractionMessageComponent:
		// Handle button interactions
		// This is handled separately by the pagination collector
	}
}

func extractUsernameFromURL(input string) string {
	matches := fanslyURLRegex.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1]
	}

	if len(input) > 0 && input[0] == '@' {
		return input[1:]
	}

	return input
}

func (b *Bot) handleAddCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	rawUsername := options[0].StringValue()

	username := extractUsernameFromURL(rawUsername)

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

	// Log what creators are being monitored
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

	if accountInfo == nil {
		log.Printf("Invalid account info structure for %s", username)
		b.respondToInteraction(s, i, "Error: Invalid account info structure", false)
		return
	}

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
					}
				}
			}
		}

		timelinePosts, timelineErr = b.APIClient.GetTimelinePost(accountInfo.ID)
		timelineAccessible = timelineErr == nil
	}

	if !timelineAccessible {
		// Create confirmation message
		confirmMsg := fmt.Sprintf("Cannot access timeline for %s. Would you like to add this account for live notifications only? "+
			"React with ✅ to add for live notifications only, or ❌ to cancel.\n"+
			"This confirmation will expire in 60 seconds.", username)

		// Send confirmation message
		msg, err := s.ChannelMessageSend(i.ChannelID, confirmMsg)
		if err != nil {
			b.respondToInteraction(s, i, "Error sending confirmation message", true)
			return
		}

		// Add reactions
		s.MessageReactionAdd(i.ChannelID, msg.ID, "✅")
		s.MessageReactionAdd(i.ChannelID, msg.ID, "❌")

		// Acknowledge the command interaction
		b.respondToInteraction(s, i, "Please respond to the confirmation message.", true)

		// Set up a timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Create a channel for the reaction response
		reactionChan := make(chan string)

		// Add temporary handler for reactions
		handlerID := s.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
			// Only process reactions to our message from the command initiator
			if r.MessageID != msg.ID || r.UserID == s.State.User.ID || r.UserID != i.Member.User.ID {
				return
			}

			switch r.Emoji.Name {
			case "✅", "❌":
				select {
				case reactionChan <- r.Emoji.Name:
				default:
				}
			}
		})

		// Wait for either a reaction or timeout
		var reaction string
		select {
		case reaction = <-reactionChan:
			// Process the reaction
		case <-ctx.Done():
			// Timeout occurred
			s.ChannelMessageEdit(i.ChannelID, msg.ID, "Confirmation timed out. Please try the command again.")
			s.ChannelMessageDelete(i.ChannelID, msg.ID)
			handlerID()
			return
		}

		// Remove the handler
		handlerID()

		// Process the reaction
		switch reaction {
		case "✅":
			// Store user with posts disabled
			repo := database.NewRepository()
			user := &models.MonitoredUser{
				GuildID:                 i.GuildID,
				UserID:                  accountInfo.ID,
				Username:                username,
				NotificationChannel:     channel.ID,
				PostNotificationChannel: channel.ID,
				LiveNotificationChannel: channel.ID,
				LastPostID:              "",
				LastStreamStart:         0,
				MentionRole:             mentionRole,
				AvatarLocation:          avatarLocation,
				AvatarLocationUpdatedAt: time.Now().Unix(),
				LiveImageURL:            "",
				PostsEnabled:            false,
				LiveEnabled:             true,
				LiveMentionRole:         mentionRole,
				PostMentionRole:         mentionRole,
			}

			err := repo.AddOrUpdateMonitoredUser(user)
			if err != nil {
				s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("Error storing user: %v", err))
				return
			}
			s.ChannelMessageEdit(i.ChannelID, msg.ID, fmt.Sprintf("Added %s to monitoring list (live notifications only)", username))

		case "❌":
			s.ChannelMessageEdit(i.ChannelID, msg.ID, "Operation cancelled.")
		}

		// Delete the message after a short delay
		time.Sleep(5 * time.Second)
		s.ChannelMessageDelete(i.ChannelID, msg.ID)
		return
	}

	// Store the monitored user in the database
	repo := database.NewRepository()
	user := &models.MonitoredUser{
		GuildID:                 i.GuildID,
		UserID:                  accountInfo.ID,
		Username:                username,
		NotificationChannel:     channel.ID,
		PostNotificationChannel: channel.ID,
		LiveNotificationChannel: channel.ID,
		LastPostID:              "",
		LastStreamStart:         0,
		MentionRole:             mentionRole,
		AvatarLocation:          avatarLocation,
		AvatarLocationUpdatedAt: time.Now().Unix(),
		LiveImageURL:            "",
		PostsEnabled:            true,
		LiveEnabled:             true,
		LiveMentionRole:         mentionRole,
		PostMentionRole:         mentionRole,
	}

	err = repo.AddOrUpdateMonitoredUser(user)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error storing user: %v", err), false)
		return
	}

	b.respondToInteraction(s, i, fmt.Sprintf("Added %s to monitoring list", username), false)
}

func (b *Bot) handleRemoveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	username := i.ApplicationCommandData().Options[0].StringValue()

	// Remove the monitored user from the database
	repo := database.NewRepository()
	err := repo.DeleteMonitoredUserByUsername(i.GuildID, username)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error removing user: %v", err), false)
		return
	}

	b.respondToInteraction(s, i, fmt.Sprintf("Removed %s from monitoring list", username), false)
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
	// Get the requested page if provided
	requestedPage := 1
	if len(i.ApplicationCommandData().Options) > 0 {
		requestedPage = int(i.ApplicationCommandData().Options[0].IntValue())
		requestedPage = max(1, requestedPage)
	}

	// Fetch monitored users for the current guild
	repo := database.NewRepository()
	users, err := repo.GetMonitoredUsersForGuild(i.GuildID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error fetching monitored users: %v", err), false)
		return
	}

	var monitoredUsers []string
	for _, user := range users {
		// Directly use the channel IDs for proper Discord mention formatting
		postChannelInfo := fmt.Sprintf("<#%s>", user.PostNotificationChannel)
		liveChannelInfo := fmt.Sprintf("<#%s>", user.LiveNotificationChannel)
		roleInfoPost := getRoleName(s, i.GuildID, user.PostMentionRole)
		roleInfoLive := getRoleName(s, i.GuildID, user.LiveMentionRole)

		// Create status indicators
		postStatus := "✅"
		if !user.PostsEnabled {
			postStatus = "❌"
		}
		liveStatus := "✅"
		if !user.LiveEnabled {
			liveStatus = "❌"
		}

		userInfo := fmt.Sprintf("- %s\n  • Post Channel: %s\n  • Live Channel: %s\n  • Live Role: %s\n  • Post Role: %s\n  • Posts: %s\n  • Live: %s",
			user.Username,
			postChannelInfo,
			liveChannelInfo,
			roleInfoLive,
			roleInfoPost,
			postStatus,
			liveStatus,
		)
		monitoredUsers = append(monitoredUsers, userInfo)
	}

	if len(monitoredUsers) == 0 {
		b.respondToInteraction(s, i, "No models are currently being monitored.", false)
		return
	}

	// Create paginated response with the requested page
	b.sendPaginatedList(s, i, monitoredUsers, requestedPage)
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
			break
		}
	}

	if imageURL == "" {
		b.editInteractionResponse(s, i, "Please attach an image to set as the live image.")
		return
	}

	// Update the database with the new live image URL
	repo := database.NewRepository()
	err = repo.UpdateLiveImageURL(i.GuildID, username, imageURL)
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

	repo := database.NewRepository()
	var err error

	switch notifiType {
	case "posts":
		if enabled {
			err = repo.EnablePostsByUsername(i.GuildID, username)
		} else {
			err = repo.DisablePostsByUsername(i.GuildID, username)
		}
	case "live":
		if enabled {
			err = repo.EnableLiveByUsername(i.GuildID, username)
		} else {
			err = repo.DisableLiveByUsername(i.GuildID, username)
		}
	default:
		b.respondToInteraction(s, i, "Invalid notification type", false)
		return
	}

	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating settings: %v", err), false)
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

	repo := database.NewRepository()
	var err error

	switch notifType {
	case "posts":
		err = repo.UpdatePostChannel(i.GuildID, username, channel.ID)
	case "live":
		err = repo.UpdateLiveChannel(i.GuildID, username, channel.ID)
	default:
		b.respondToInteraction(s, i, "Invalid notification type", false)
		return
	}

	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating channel: %v", err), false)
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

	repo := database.NewRepository()
	err := repo.UpdatePostMentionRole(i.GuildID, username, roleID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating post mention role: %v", err), false)
		return
	}

	message := fmt.Sprintf("Post mention role for %s has been cleared.", username)
	if roleID != "" {
		message = fmt.Sprintf("Post mention role for %s set to <@&%s>", username, roleID)
	}
	b.respondToInteraction(s, i, message, false)
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

	repo := database.NewRepository()
	err := repo.UpdateLiveMentionRole(i.GuildID, username, roleID)
	if err != nil {
		b.respondToInteraction(s, i, fmt.Sprintf("Error updating live mention role: %v", err), false)
		return
	}

	message := fmt.Sprintf("Live mention role for %s has been cleared.", username)
	if roleID != "" {
		message = fmt.Sprintf("Live mention role for %s set to <@&%s>", username, roleID)
	}
	b.respondToInteraction(s, i, message, false)
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

func (b *Bot) editInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		log.Printf("Error editing interaction response: %v", err)
	}
}

func (b *Bot) hasAdminOrModPermissions(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	// return false if interaction is in dms
	if i.GuildID == "" {
		return false
	}

	// Check if Member is nil (can happen in some interaction types)
	if i.Member == nil {
		// Try to get member info if possible
		member, err := s.GuildMember(i.GuildID, i.User.ID)
		if err != nil {
			log.Printf("Error fetching member info: %v", err)
			return false
		}
		i.Member = member
	}

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
