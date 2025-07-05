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
		if !b.hasAdminOrModPermissions(s, i) {
			username := "User"
			if i.User != nil {
				username = i.User.Username
			} else if i.Member != nil && i.Member.User != nil {
				username = i.Member.User.Username
			}
			b.respondToInteraction(s, i, "You do not have permission to use this command.", true)
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
		// Button interactions are handled by the pagination collector
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

	// Check if the limit is enabled (a value > 0)
	if config.MaxMonitoredUsersPerGuild > 0 {
		// Get the current count of monitored users for this guild
		count, err := b.Repo.CountMonitoredUsersForGuild(i.GuildID)
		if err != nil {
			log.Printf("Error checking guild limit for guild %s: %v", i.GuildID, err)
			b.respondToInteraction(s, i, "An error occurred while checking the server's limit. Please try again later.", true)
			return
		}

		// Check if the count has reached or exceeded the limit
		if count >= int64(config.MaxMonitoredUsersPerGuild) {
			// To avoid blocking updates for users who are already being monitored,
			// we can perform a quick check. This is an optional optimization.
			// First, try to find the user by username in this guild.
			existingUser, _ := b.Repo.GetMonitoredUserByUsername(i.GuildID, username)
			if existingUser == nil {
				// The user does not exist, and the guild is at its limit, so we block the addition.
				message := fmt.Sprintf("This server has reached its limit of %d monitored users. To add another, you must first remove one using `/remove`.", config.MaxMonitoredUsersPerGuild)
				b.respondToInteraction(s, i, message, true)
				return
			}
			// If existingUser is not nil, it means they are just updating an existing entry, so we allow it to proceed.
		}
	}

	if tokenRegex.MatchString(username) {
		b.respondToInteraction(s, i, "Error: Username appears to contain a token. Please provide a valid username.", true)
		return
	}

	// Defer the response to prevent a timeout. The final response will be public.
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	// Run all long-running tasks in a goroutine so the handler returns immediately.
	go func() {
		channel := options[1].ChannelValue(s)
		var mentionRole string
		if len(options) > 2 {
			if role := options[2].RoleValue(s, i.GuildID); role != nil {
				mentionRole = role.ID
			}
		}

		logMessage := fmt.Sprintf("`[ %s ]` %s:\n`Requested Creator Name:` **%s**\n----------",
			time.Now().Format("2006-01-02 15:04:05"),
			i.Member.User.Username,
			username,
		)
		_, logErr := s.ChannelMessageSend(config.LogChannelID, logMessage)
		if logErr != nil {
			log.Printf("Failed to send log message: %v", logErr)
		}

		accountInfo, err := b.APIClient.GetAccountInfo(username)
		if err != nil {
			log.Printf("Error getting account info for %s: %v", username, err)
			b.editInteractionResponse(s, i, fmt.Sprintf("Error fetching account info: The user might not exist or Fansly API is unavailable. (%v)", err))
			return
		}

		if accountInfo == nil {
			log.Printf("Invalid account info structure for %s", username)
			b.editInteractionResponse(s, i, "Error: Could not retrieve valid account info for this user.")
			return
		}

		var avatarLocation string
		if len(accountInfo.Avatar.Variants) > 0 && len(accountInfo.Avatar.Variants[0].Locations) > 0 {
			avatarLocation = accountInfo.Avatar.Variants[0].Locations[0].Location
		} else {
			log.Printf("Warning: No avatar found for user %s", username)
		}

		timelinePosts, timelineErr := b.APIClient.GetTimelinePost(accountInfo.ID)
		timelineAccessible := timelineErr == nil && len(timelinePosts) >= 0

		if !timelineAccessible {
			// Try to follow the account to gain access
			if myAccount, err := b.APIClient.GetMyAccountInfo(); err == nil && myAccount.ID != "" {
				if following, err := b.APIClient.GetFollowing(myAccount.ID); err == nil {
					isFollowing := false
					for _, f := range following {
						if f.AccountID == accountInfo.ID {
							isFollowing = true
							break
						}
					}
					if !isFollowing {
						if followErr := b.APIClient.FollowAccount(accountInfo.ID); followErr != nil {
							log.Printf("Note: Could not automatically follow %s: %v", username, followErr)
						}
					}
				}
			}
			timelinePosts, timelineErr = b.APIClient.GetTimelinePost(accountInfo.ID)
			timelineAccessible = timelineErr == nil
		}

		if !timelineAccessible {
			b.editInteractionResponse(s, i, fmt.Sprintf("Cannot access timeline for **%s**. A confirmation message has been sent below.", username))

			confirmMsgContent := fmt.Sprintf("%s, do you want to add **%s** for **live notifications only**? React with ✅ to confirm or ❌ to cancel.", i.Member.Mention(), username)
			msg, err := s.ChannelMessageSend(i.ChannelID, confirmMsgContent)
			if err != nil {
				log.Printf("Error sending confirmation message: %v", err)
				return
			}
			s.MessageReactionAdd(i.ChannelID, msg.ID, "✅")
			s.MessageReactionAdd(i.ChannelID, msg.ID, "❌")

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			reactionChan := make(chan string)
			handlerID := s.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
				if r.MessageID != msg.ID || r.UserID == s.State.User.ID || r.UserID != i.Member.User.ID {
					return
				}
				if r.Emoji.Name == "✅" || r.Emoji.Name == "❌" {
					select {
					case reactionChan <- r.Emoji.Name:
					default:
					}
				}
			})
			defer handlerID()

			select {
			case reaction := <-reactionChan:
				if reaction == "✅" {
					user := &models.MonitoredUser{
						GuildID: i.GuildID, UserID: accountInfo.ID, Username: username, NotificationChannel: channel.ID, PostNotificationChannel: channel.ID,
						LiveNotificationChannel: channel.ID, LastPostID: "", LastStreamStart: 0, MentionRole: mentionRole, AvatarLocation: avatarLocation,
						AvatarLocationUpdatedAt: time.Now().Unix(), LiveImageURL: "", PostsEnabled: false, LiveEnabled: true, LiveMentionRole: mentionRole, PostMentionRole: mentionRole,
					}
					if err := database.NewRepository().AddOrUpdateMonitoredUser(user); err != nil {
						s.ChannelMessageEdit(i.ChannelID, msg.ID, fmt.Sprintf("Error adding user: %v", err))
					} else {
						s.ChannelMessageEdit(i.ChannelID, msg.ID, fmt.Sprintf("✅ Added **%s** for live notifications only.", username))
					}
				} else {
					s.ChannelMessageEdit(i.ChannelID, msg.ID, "❌ Operation cancelled.")
				}
			case <-ctx.Done():
				s.ChannelMessageEdit(i.ChannelID, msg.ID, "Confirmation timed out.")
			}
			time.AfterFunc(10*time.Second, func() { s.ChannelMessageDelete(i.ChannelID, msg.ID) })
			return
		}

		repo := database.NewRepository()
		user := &models.MonitoredUser{
			GuildID: i.GuildID, UserID: accountInfo.ID, Username: username, NotificationChannel: channel.ID, PostNotificationChannel: channel.ID,
			LiveNotificationChannel: channel.ID, LastPostID: "", LastStreamStart: 0, MentionRole: mentionRole, AvatarLocation: avatarLocation,
			AvatarLocationUpdatedAt: time.Now().Unix(), LiveImageURL: "", PostsEnabled: true, LiveEnabled: true, LiveMentionRole: mentionRole, PostMentionRole: mentionRole,
		}

		err = repo.AddOrUpdateMonitoredUser(user)
		if err != nil {
			b.editInteractionResponse(s, i, fmt.Sprintf("Error storing user in database: %v", err))
			return
		}

		b.editInteractionResponse(s, i, fmt.Sprintf("Successfully added **%s** to the monitoring list for all notifications.", username))
	}()
}

func (b *Bot) handleRemoveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	username := i.ApplicationCommandData().Options[0].StringValue()

	repo := database.NewRepository()
	err = repo.DeleteMonitoredUserByUsername(i.GuildID, username)
	if err != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error removing user: %v", err))
		return
	}

	b.editInteractionResponse(s, i, fmt.Sprintf("Removed **%s** from the monitoring list.", username))
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
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	requestedPage := 1
	if len(i.ApplicationCommandData().Options) > 0 {
		requestedPage = int(i.ApplicationCommandData().Options[0].IntValue())
		requestedPage = max(1, requestedPage)
	}

	repo := database.NewRepository()
	users, err := repo.GetMonitoredUsersForGuild(i.GuildID)
	if err != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error fetching monitored users: %v", err))
		return
	}

	if len(users) == 0 {
		b.editInteractionResponse(s, i, "No models are currently being monitored.")
		return
	}

	var monitoredUsers []string
	for _, user := range users {
		postChannelInfo := fmt.Sprintf("<#%s>", user.PostNotificationChannel)
		liveChannelInfo := fmt.Sprintf("<#%s>", user.LiveNotificationChannel)
		roleInfoPost := getRoleName(user.PostMentionRole)
		roleInfoLive := getRoleName(user.LiveMentionRole)

		postStatus := "✅ Enabled"
		if !user.PostsEnabled {
			postStatus = "❌ Disabled"
		}
		liveStatus := "✅ Enabled"
		if !user.LiveEnabled {
			liveStatus = "❌ Disabled"
		}

		userInfo := fmt.Sprintf("- **%s**\n  • Posts: %s (in %s | Role: %s)\n  • Live: %s (in %s | Role: %s)",
			user.Username,
			postStatus, postChannelInfo, roleInfoPost,
			liveStatus, liveChannelInfo, roleInfoLive,
		)
		monitoredUsers = append(monitoredUsers, userInfo)
	}

	b.sendPaginatedList(s, i, monitoredUsers, requestedPage)
}

func (b *Bot) handleSetLiveImageCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error acknowledging interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()

	var imageURL string
	if attachments := i.ApplicationCommandData().Resolved.Attachments; len(attachments) > 0 {
		for _, attachment := range attachments {
			imageURL = attachment.URL
			break
		}
	}

	if imageURL == "" {
		b.editInteractionResponse(s, i, "Please attach an image to set as the live image.")
		return
	}

	repo := database.NewRepository()
	err = repo.UpdateLiveImageURL(i.GuildID, username, imageURL)
	if err != nil {
		log.Printf("Error updating live image URL: %v", err)
		b.editInteractionResponse(s, i, fmt.Sprintf("An error occurred while setting the live image: %v", err))
		return
	}

	b.editInteractionResponse(s, i, fmt.Sprintf("Live image for **%s** has been set successfully.", username))
}

func (b *Bot) handleToggleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	notifiType := options[1].StringValue()
	enabled := options[2].BoolValue()

	repo := database.NewRepository()
	var updateErr error

	switch notifiType {
	case "posts":
		if enabled {
			updateErr = repo.EnablePostsByUsername(i.GuildID, username)
		} else {
			updateErr = repo.DisablePostsByUsername(i.GuildID, username)
		}
	case "live":
		if enabled {
			updateErr = repo.EnableLiveByUsername(i.GuildID, username)
		} else {
			updateErr = repo.DisableLiveByUsername(i.GuildID, username)
		}
	default:
		b.editInteractionResponse(s, i, "Invalid notification type selected.")
		return
	}

	if updateErr != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error updating settings: %v", updateErr))
		return
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}

	b.editInteractionResponse(s, i, fmt.Sprintf("`%s` notifications have been **%s** for **%s**.", notifiType, status, username))
}

func (b *Bot) handleSetChannelCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	notifType := options[1].StringValue()
	channel := options[2].ChannelValue(s)

	repo := database.NewRepository()
	var updateErr error

	switch notifType {
	case "posts":
		updateErr = repo.UpdatePostChannel(i.GuildID, username, channel.ID)
	case "live":
		updateErr = repo.UpdateLiveChannel(i.GuildID, username, channel.ID)
	default:
		b.editInteractionResponse(s, i, "Invalid notification type.")
		return
	}

	if updateErr != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error updating channel: %v", updateErr))
		return
	}

	b.editInteractionResponse(s, i, fmt.Sprintf("Successfully set the %s notification channel for **%s** to %s.", notifType, username, channel.Mention()))
}

func (b *Bot) handleSetPostMentionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	var roleID string
	var roleMention string

	if len(options) > 1 {
		role := options[1].RoleValue(s, i.GuildID)
		if role != nil {
			roleID = role.ID
			roleMention = role.Mention()
		}
	}

	repo := database.NewRepository()
	err = repo.UpdatePostMentionRole(i.GuildID, username, roleID)
	if err != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error updating post mention role: %v", err))
		return
	}

	message := fmt.Sprintf("Post mention role for **%s** has been cleared.", username)
	if roleID != "" {
		message = fmt.Sprintf("Post mention role for **%s** set to %s.", username, roleMention)
	}
	b.editInteractionResponse(s, i, message)
}

func (b *Bot) handleSetLiveMentionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	var roleID string
	var roleMention string

	if len(options) > 1 {
		role := options[1].RoleValue(s, i.GuildID)
		if role != nil {
			roleID = role.ID
			roleMention = role.Mention()
		}
	}

	repo := database.NewRepository()
	err = repo.UpdateLiveMentionRole(i.GuildID, username, roleID)
	if err != nil {
		b.editInteractionResponse(s, i, fmt.Sprintf("Error updating live mention role: %v", err))
		return
	}

	message := fmt.Sprintf("Live mention role for **%s** has been cleared.", username)
	if roleID != "" {
		message = fmt.Sprintf("Live mention role for **%s** set to %s.", username, roleMention)
	}
	b.editInteractionResponse(s, i, message)
}

func getRoleName(roleID string) string {
	if roleID == "" || roleID == "0" {
		return "None"
	}
	// Use role mention for clickable link
	return fmt.Sprintf("<@&%s>", roleID)
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
	if i.GuildID == "" {
		return false // No permissions in DMs
	}

	// Check for administrator permission
	if i.Member.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true
	}

	if i.Member.Permissions&discordgo.PermissionManageGuild == discordgo.PermissionManageGuild {
		return true
	}

	// Check if the user is the server owner
	guild, err := s.State.Guild(i.GuildID)
	if err == nil && guild.OwnerID == i.Member.User.ID {
		return true
	}

	return false
}
