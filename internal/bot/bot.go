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
	// This event fires when the bot is kicked, banned, or the guild is deleted.
	// If event.Unavailable is true, it means a Discord outage, so we shouldn't delete data.
	if !event.Unavailable {
		log.Printf("Bot removed from guild: %s. Cleaning up associated data.", event.ID)
		err := b.Repo.DeleteAllUsersInGuild(event.ID)
		if err != nil {
			log.Printf("Error deleting users for guild %s: %v", event.ID, err)
		} else {
			log.Printf("Successfully cleaned up data for guild %s", event.ID)
		}
	} else {
		log.Printf("Guild %s became unavailable.", event.ID)
	}

	b.updateBotStatus()
}

func (b *Bot) monitorUsers() {
	ticker := time.NewTicker(time.Duration(config.MonitorIntervalSeconds) * time.Second)
	defer ticker.Stop()

	numWorkers := config.MonitorWorkerCount
	if numWorkers <= 0 {
		numWorkers = 1 // Ensure at least one worker.
	}
	jobs := make(chan []models.MonitoredUser, 100) // Buffered channel

	// Start long-lived workers that will process jobs as they come in.
	for w := 1; w <= numWorkers; w++ {
		go b.worker(w, jobs)
	}

	// Run the first check immediately on bot start, then on every tick.
	log.Println("Dispatching initial monitoring cycle...")
	b.dispatchMonitoringJobs(jobs)

	for range ticker.C {
		b.dispatchMonitoringJobs(jobs)
	}
}

func (b *Bot) dispatchMonitoringJobs(jobs chan<- []models.MonitoredUser) {
	users, err := b.Repo.GetMonitoredUsers()
	if err != nil {
		log.Printf("Error getting monitored users: %v", err)
		return
	}

	// Group users by UserID to deduplicate API calls
	userGroups := make(map[string][]models.MonitoredUser)
	for _, user := range users {
		userGroups[user.UserID] = append(userGroups[user.UserID], user)
	}

	log.Printf("Dispatching %d unique users to %d workers.", len(userGroups), config.MonitorWorkerCount)

	// Send each group of users as a single job to the workers channel.
	for _, userEntries := range userGroups {
		jobs <- userEntries
	}
}

// New worker function in bot.go
func (b *Bot) worker(id int, jobs <-chan []models.MonitoredUser) {
	avatarRefreshDuration := int64(config.AvatarRefreshIntervalHours * 60 * 60)

	for userEntries := range jobs {
		primaryUser := userEntries[0]

		// Check if avatar needs refreshing
		if time.Now().Unix()-primaryUser.AvatarLocationUpdatedAt > avatarRefreshDuration {
			newAvatarLocation, err := b.refreshAvatarURL(primaryUser.Username)
			if err != nil {
				log.Printf("[Worker %d] Error refreshing avatar URL for %s: %v", id, primaryUser.Username, err)
			} else {
				// Update avatar for all entries of this user
				for _, user := range userEntries {
					err = b.Repo.UpdateAvatarInfo(user.GuildID, user.UserID, newAvatarLocation)
					if err != nil {
						log.Printf("[Worker %d] Error updating avatar URL in DB for %s in guild %s: %v", id, user.Username, user.GuildID, err)
					}
				}
				// Update the avatar in memory for the current cycle's checks
				for i := range userEntries {
					userEntries[i].AvatarLocation = newAvatarLocation
				}
			}
		}

		// Check live stream and posts. These API calls now happen in parallel for different users.
		b.checkUserLiveStreamOptimized(userEntries)
		b.checkUserPostsOptimized(userEntries)
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

	// Make API call only once per unique user ID
	primaryUser := postEnabledUsers[0]
	latestPosts, err := b.APIClient.GetTimelinePost(primaryUser.UserID)
	if err != nil {
		log.Printf("Error fetching post info for %s: %v", primaryUser.Username, err)
		return
	}

	// If there are no posts on the timeline at all, do nothing.
	if len(latestPosts) == 0 {
		return
	}

	latestPost := latestPosts[0]

	// Now, iterate through each server monitoring this user
	for _, user := range postEnabledUsers {
		// Check if this specific server has seen this post yet.
		if latestPost.ID != user.LastPostID {
			// This server needs a notification. First, update its state.
			err := b.Repo.UpdateLastPostID(user.GuildID, user.UserID, latestPost.ID)
			if err != nil {
				log.Printf("Error updating last post ID for %s in guild %s: %v", user.Username, user.GuildID, err)
				continue // Skip this server if DB update fails
			}

			// This flag is still useful for logging, but we won't use it to suppress the ping.
			isFirstPostForThisServer := user.LastPostID == "" || user.LastPostID == "0"

			// Pass nil for postMedia, as we are no longer fetching it.
			embedMsg := embed.CreatePostEmbed(user.Username, latestPost, user.AvatarLocation, nil)

			mention := "@everyone"
			if user.PostMentionRole != "" {
				mention = fmt.Sprintf("<@&%s>", user.PostMentionRole)
			}

			targetChannel := user.PostNotificationChannel
			if targetChannel == "" {
				targetChannel = user.NotificationChannel
			}

			log.Printf("Sending post notification for %s to guild %s. First post: %t", user.Username, user.GuildID, isFirstPostForThisServer)

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
	ticker := time.NewTicker(time.Duration(config.StatusUpdateIntervalMinutes) * time.Minute)
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
