package bot

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/api"
	"github.com/fvckgrimm/discord-fansly-notify/internal/config"
	"github.com/fvckgrimm/discord-fansly-notify/internal/database"
	"github.com/fvckgrimm/discord-fansly-notify/internal/embed"
	"github.com/fvckgrimm/discord-fansly-notify/internal/models"
)

type Bot struct {
	Session   *discordgo.Session
	APIClient *api.Client
	Repo      *database.Repository
}

func New() (*Bot, error) {
	discord, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		return nil, err
	}

	apiClient, _ := api.NewClient(config.FanslyToken, config.UserAgent)

	bot := &Bot{
		Session:   discord,
		APIClient: apiClient,
		Repo:      database.NewRepository(),
	}

	bot.registerHandlers()

	return bot, nil
}

func (b *Bot) Start() error {
	err := b.Session.Open()
	if err != nil {
		return err
	}

	go b.monitorUsers()
	go b.updateStatusPeriodically()

	return nil
}

func (b *Bot) Stop() {
	b.Session.Close()
}

func (b *Bot) registerHandlers() {
	b.Session.AddHandler(b.ready)
	b.Session.AddHandler(b.interactionCreate)
	b.Session.AddHandler(b.guildCreate)
	b.Session.AddHandler(b.guildDelete)
}

func (b *Bot) guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	log.Printf("Bot joined a new server: %s", event.Guild.Name)
	b.updateBotStatus()
}

func (b *Bot) guildDelete(s *discordgo.Session, event *discordgo.GuildDelete) {
	log.Printf("Bot left a server: %s", event.Guild.Name)
	b.updateBotStatus()
}

func (b *Bot) monitorUsers() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		users, err := b.Repo.GetMonitoredUsers()
		if err != nil {
			log.Printf("Error getting monitored users: %v", err)
			continue
		}

		// Group users by UserID to deduplicate API calls
		userGroups := make(map[string][]models.MonitoredUser)
		for _, user := range users {
			userGroups[user.UserID] = append(userGroups[user.UserID], user)
		}

		// Process each unique user only once
		for _, userEntries := range userGroups {
			// Use the first entry to get user info (they should all have same UserID/Username)
			primaryUser := userEntries[0]

			// Check if avatar needs refreshing (only once per user)
			if time.Now().Unix()-primaryUser.AvatarLocationUpdatedAt > 6*24*60*60 {
				newAvatarLocation, err := b.refreshAvatarURL(primaryUser.Username)
				if err != nil {
					log.Printf("Error refreshing avatar URL for user %s: %v", primaryUser.Username, err)
				} else {
					// Update avatar for all entries of this user
					for _, user := range userEntries {
						err = b.Repo.UpdateAvatarInfo(user.GuildID, user.UserID, newAvatarLocation)
						if err != nil {
							log.Printf("Error updating avatar URL in database: %v", err)
						}
					}
				}
			}

			// Check live stream only once per user
			b.checkUserLiveStreamOptimized(userEntries)

			// Check posts only once per user
			b.checkUserPostsOptimized(userEntries)
		}
	}
}

func (b *Bot) checkUserLiveStreamOptimized(userEntries []models.MonitoredUser) {
	// Filter entries that have live notifications enabled
	liveEnabledUsers := make([]models.MonitoredUser, 0)
	for _, user := range userEntries {
		if user.LiveEnabled {
			liveEnabledUsers = append(liveEnabledUsers, user)
		}
	}

	if len(liveEnabledUsers) == 0 {
		return
	}

	// Make API call only once
	primaryUser := liveEnabledUsers[0]
	streamInfo, err := b.APIClient.GetStreamInfo(primaryUser.UserID)
	if err != nil {
		log.Printf("Error fetching stream info for %s: %v", primaryUser.Username, err)
		return
	}

	// Check if it's a new stream
	if streamInfo.Response.Stream.Status == 2 && streamInfo.Response.Stream.StartedAt > primaryUser.LastStreamStart {
		// Send notifications to all servers that have this user monitored with live enabled
		for _, user := range liveEnabledUsers {
			err = b.Repo.UpdateLastStreamStart(user.GuildID, user.UserID, streamInfo.Response.Stream.StartedAt)
			if err != nil {
				log.Printf("Error updating last stream start: %v", err)
				continue
			}

			embedMsg := embed.CreateLiveStreamEmbed(user.Username, streamInfo, user.AvatarLocation, user.LiveImageURL)
			mention := "@everyone"
			if user.LiveMentionRole != "" {
				mention = fmt.Sprintf("<@&%s>", user.LiveMentionRole)
			}

			targetChannel := user.LiveNotificationChannel
			if targetChannel == "" {
				targetChannel = user.NotificationChannel
			}

			_, err = b.Session.ChannelMessageSendComplex(targetChannel, &discordgo.MessageSend{
				Content: mention,
				Embed:   embedMsg,
			})
			if err != nil {
				b.logNotificationError("live stream", user, targetChannel, err)
			}
		}
	}
}

func (b *Bot) checkUserPostsOptimized(userEntries []models.MonitoredUser) {
	// Filter entries that have post notifications enabled
	postEnabledUsers := make([]models.MonitoredUser, 0)
	for _, user := range userEntries {
		if user.PostsEnabled {
			postEnabledUsers = append(postEnabledUsers, user)
		}
	}

	if len(postEnabledUsers) == 0 {
		return
	}

	// Make API call only once
	primaryUser := postEnabledUsers[0]
	postInfo, err := b.APIClient.GetTimelinePost(primaryUser.UserID)
	if err != nil {
		log.Printf("Error fetching post info for %s: %v", primaryUser.Username, err)
		return
	}

	if len(postInfo) > 0 && postInfo[0].ID != primaryUser.LastPostID {
		// Fetch post media once
		postMedia, err := b.APIClient.GetPostMedia(postInfo[0].ID, b.APIClient.Token, b.APIClient.UserAgent)
		if err != nil {
			log.Printf("Error fetching post media: %v", err)
		}

		// Send notifications to all servers that have this user monitored with posts enabled
		for _, user := range postEnabledUsers {
			err = b.Repo.UpdateLastPostID(user.GuildID, user.UserID, postInfo[0].ID)
			if err != nil {
				log.Printf("Error updating last post ID: %v", err)
				continue
			}

			embedMsg := embed.CreatePostEmbed(user.Username, postInfo[0], user.AvatarLocation, postMedia)
			mention := "@everyone"
			if user.PostMentionRole != "" {
				mention = fmt.Sprintf("<@&%s>", user.PostMentionRole)
			}

			targetChannel := user.PostNotificationChannel
			if targetChannel == "" {
				targetChannel = user.NotificationChannel
			}

			_, err = b.Session.ChannelMessageSendComplex(targetChannel, &discordgo.MessageSend{
				Content: mention,
				Embed:   embedMsg,
			})
			if err != nil {
				b.logNotificationError("post", user, targetChannel, err)
			}
		}
	}
}

func (b *Bot) logNotificationError(notificationType string, user models.MonitoredUser, targetChannel string, err error) {
	guild, _ := b.Session.Guild(user.GuildID)
	guildName := "Unknown Server"
	if guild != nil {
		guildName = guild.Name
	}
	channel, _ := b.Session.Channel(targetChannel)
	channelName := "Unknown Channel"
	if channel != nil {
		channelName = channel.Name
	}
	log.Printf("Error sending %s notification for %s | Server: %s (%s) | Channel: %s (%s) | Error: %v",
		notificationType,
		user.Username,
		guildName,
		user.GuildID,
		channelName,
		targetChannel,
		err,
	)
}

func (b *Bot) updateStatusPeriodically() {
	ticker := time.NewTicker(120 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		b.updateBotStatus()
	}
}

func (b *Bot) refreshAvatarURL(username string) (string, error) {
	accountInfo, err := b.APIClient.GetAccountInfo(username)
	if err != nil {
		return "", err
	}

	if accountInfo == nil || accountInfo.Avatar.Locations == nil || len(accountInfo.Avatar.Variants) == 0 || len(accountInfo.Avatar.Variants[0].Locations) == 0 {
		return "", fmt.Errorf("invalid account info structure for user %s", username)
	}

	return accountInfo.Avatar.Variants[0].Locations[0].Location, nil
}

func (b *Bot) updateBotStatus() {
	serverCount := len(b.Session.State.Guilds)
	status := fmt.Sprintf("Watching %d servers", serverCount)
	err := b.Session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: status,
				Type: discordgo.ActivityTypeWatching,
			},
		},
	})
	if err != nil {
		log.Printf("Error updating status: %v", err)
	}
}
