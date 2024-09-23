package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/api"
	"github.com/fvckgrimm/discord-fansly-notify/internal/config"
	"github.com/fvckgrimm/discord-fansly-notify/internal/database"
	"github.com/fvckgrimm/discord-fansly-notify/internal/embed"
)

type Bot struct {
	Session   *discordgo.Session
	APIClient *api.Client
	DB        *sql.DB
}

func New() (*Bot, error) {
	discord, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		return nil, err
	}

	apiClient := api.NewClient(config.FanslyToken, config.UserAgent)

	bot := &Bot{
		Session:   discord,
		APIClient: apiClient,
		DB:        database.DB,
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
		err := b.retryDbOperation(func() error {
			tx, err := b.DB.BeginTx(context.Background(), nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			rows, err := tx.Query("SELECT guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at, live_image_url FROM monitored_users")
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var user struct {
					GuildID                 string
					UserID                  string
					Username                string
					NotificationChannel     string
					LastPostID              string
					LastStreamStart         int64
					MentionRole             string
					AvatarLocation          string
					AvatarLocationUpdatedAt int64
					LiveImageURL            string
				}

				err := rows.Scan(&user.GuildID, &user.UserID, &user.Username, &user.NotificationChannel, &user.LastPostID, &user.LastStreamStart, &user.MentionRole, &user.AvatarLocation, &user.AvatarLocationUpdatedAt, &user.LiveImageURL)
				if err != nil {
					log.Printf("Error scanning row: %v", err)
					continue
				}

				// Check for new posts and live streams
				streamInfo, err := b.APIClient.GetStreamInfo(user.UserID)
				if err != nil {
					log.Printf("Error fetching stream info: %v", err)
					continue
				}

				// Check if avatar URL needs refreshing (e.g., older than 6 days)
				if time.Now().Unix()-user.AvatarLocationUpdatedAt > 6*24*60*60 {
					newAvatarLocation, err := b.refreshAvatarURL(user.Username)
					if err != nil {
						log.Printf("Error refreshing avatar URL for user %s: %v", user.Username, err)
					} else {
						_, err = tx.Exec(`
							UPDATE monitored_users 
							SET avatar_location = ?, avatar_location_updated_at = ?
							WHERE guild_id = ? AND user_id = ?
						`, newAvatarLocation, time.Now().Unix(), user.GuildID, user.UserID)
						if err != nil {
							log.Printf("Error updating avatar URL in database: %v", err)
						} else {
							user.AvatarLocation = newAvatarLocation
						}
					}
				}

				if streamInfo.Response.Stream.Status == 2 && streamInfo.Response.Stream.StartedAt > user.LastStreamStart {
					_, err = tx.Exec(`
						UPDATE monitored_users 
						SET last_stream_start = ? 
						WHERE guild_id = ? AND user_id = ?
					`, streamInfo.Response.Stream.StartedAt, user.GuildID, user.UserID)
					if err != nil {
						return err
					}

					embedMsg := embed.CreateLiveStreamEmbed(user.Username, streamInfo, user.AvatarLocation, user.LiveImageURL)

					mention := "@everyone"
					if user.MentionRole != "" {
						mention = fmt.Sprintf("<@&%s>", user.MentionRole)
					}

					_, err = b.Session.ChannelMessageSendComplex(user.NotificationChannel, &discordgo.MessageSend{
						Content: mention,
						Embed:   embedMsg,
					})
					if err != nil {
						log.Printf("Error sending live stream notification: %v", err)
					}
				}

				postInfo, err := b.APIClient.GetTimelinePost(user.UserID)
				if err != nil {
					log.Printf("Error fetching post info: %v", err)
					continue
				}

				if len(postInfo) > 0 && postInfo[0].ID != user.LastPostID {
					_, err = tx.Exec(`
                        UPDATE monitored_users 
                        SET last_post_id = ? 
                        WHERE guild_id = ? AND user_id = ?
                    `, postInfo[0].ID, user.GuildID, user.UserID)
					if err != nil {
						return err
					}

					// Fetch post media
					postMedia, err := b.APIClient.GetPostMedia(postInfo[0].ID, b.APIClient.Token, b.APIClient.UserAgent)
					if err != nil {
						log.Printf("Error fetching post media: %v", err)
					}

					embedMsg := embed.CreatePostEmbed(user.Username, postInfo[0], user.AvatarLocation, postMedia)

					mention := "@everyone"
					if user.MentionRole != "" {
						mention = fmt.Sprintf("<@&%s>", user.MentionRole)
					}

					_, err = b.Session.ChannelMessageSendComplex(user.NotificationChannel, &discordgo.MessageSend{
						Content: mention,
						Embed:   embedMsg,
					})
					if err != nil {
						log.Printf("Error sending post notification: %v", err)
					}
				} else if len(postInfo) == 0 {
					//log.Printf("No new posts found for user %s", user.Username)
					continue
				}
			}

			return tx.Commit()
		})

		if err != nil {
			log.Printf("Error monitoring users: %v", err)
		}
	}
}

func (b *Bot) updateStatusPeriodically() {
	ticker := time.NewTicker(120 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		b.updateBotStatus()
	}
}

func (b *Bot) retryDbOperation(operation func() error) error {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		if err.Error() == "database is locked" {
			time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
			continue
		}
		return err
	}
	return fmt.Errorf("operation failed after %d retries", maxRetries)
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
