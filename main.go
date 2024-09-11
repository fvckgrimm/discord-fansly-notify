package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	//"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/api"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

type ServerConfig struct {
	MonitoredUsers map[string]UserConfig
}

type UserConfig struct {
	Username            string
	NotificationChannel string
	LastPostID          string
	LastStreamStart     int64
	MentionRole         string
}

var (
	configs     map[string]*ServerConfig
	configMutex sync.RWMutex
	apiClient   *api.Client
	appID       string
	publicKey   string
	db          *sql.DB
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	appID = os.Getenv("APP_ID")
	publicKey = os.Getenv("PUBLIC_KEY")

	if appID == "" || publicKey == "" {
		log.Fatal("Missing required environment variables")
	}

	apiClient = api.NewClient(os.Getenv("FANSLY_TOKEN"), os.Getenv("USER_AGENT"))
	configs = make(map[string]*ServerConfig)

	// Initialize SQLite database
	//var err error
	db, err = sql.Open("sqlite", "bot.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create tables if they don't exist
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS monitored_users (
            guild_id TEXT,
            user_id TEXT,
            username TEXT,
            notification_channel TEXT,
            last_post_id TEXT,
            last_stream_start INTEGER,
			mention_role TEXT,
			avatar_location TEXT,
			avatar_location_updated_at INTEGER,
            PRIMARY KEY (guild_id, user_id)
        )
    `)
	if err != nil {
		log.Fatal(err)
	}
}

func refreshAvatarURL(userID string) (string, error) {
	accountInfo, err := apiClient.GetAccountInfo(userID)
	if err != nil {
		return "", err
	}

	if accountInfo == nil || accountInfo.Avatar.Locations == nil || len(accountInfo.Avatar.Variants) == 0 || len(accountInfo.Avatar.Variants[0].Locations) == 0 {
		return "", fmt.Errorf("invalid account info structure for user %s", userID)
	}

	return accountInfo.Avatar.Variants[0].Locations[0].Location, nil
}

func main() {
	discord, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	discord.AddHandler(ready)
	discord.AddHandler(interactionCreate)

	err = discord.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer discord.Close()

	go monitorUsers(discord)
	go updateStatusPeriodically(discord)

	log.Println("Bot is now running. Press CTRL-C to exit.")
	select {}
}

func updateStatusPeriodically(s *discordgo.Session) {
	ticker := time.NewTicker(180 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		updateBotStatus(s)
	}
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready")
	serverCount := len(s.State.Guilds)
	log.Printf("Currently active and monitoring in %d servers", serverCount)
	manageServerPerm := int64(discordgo.PermissionManageServer)

	updateBotStatus(s)

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", []*discordgo.ApplicationCommand{
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
			DefaultMemberPermissions: &manageServerPerm,
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
			DefaultMemberPermissions: &manageServerPerm,
		},
	})
	if err != nil {
		log.Printf("Error registering commands: %v", err)
	} else {
		log.Println("Commands registered successfully")
	}
}

func updateBotStatus(s *discordgo.Session) {
	serverCount := len(s.State.Guilds)
	status := fmt.Sprintf("Watching %d servers", serverCount)
	err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
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

func hasAdminOrModPermissions(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
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

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !hasAdminOrModPermissions(s, i) {
		respondToInteraction(s, i, "You do not have permission to use this command.")
		return
	}

	switch i.ApplicationCommandData().Name {
	case "add":
		handleAddCommand(s, i)
	case "remove":
		handleRemoveCommand(s, i)
	}
}

func handleAddCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	username := options[0].StringValue()
	channel := options[1].ChannelValue(s)
	var mentionRole string
	if len(options) > 2 {
		if role := options[2].RoleValue(s, i.GuildID); role != nil {
			mentionRole = role.ID
		}
	}

	accountInfo, err := apiClient.GetAccountInfo(username)
	if err != nil {
		log.Printf("Error getting account info for %s: %v", username, err)
		respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}

	if accountInfo == nil || accountInfo.Avatar.Locations == nil || len(accountInfo.Avatar.Variants) == 0 || len(accountInfo.Avatar.Variants[0].Locations) == 0 {
		log.Printf("Invalid account info structure for %s", username)
		respondToInteraction(s, i, "Error: Invalid account info structure")
		return
	}

	avatarLocation := accountInfo.Avatar.Variants[0].Locations[0].Location

	// Check if the account is already being followed
	myAccount, err := apiClient.GetMyAccountInfo()
	//fmt.Printf("%v", myAccount)
	if err != nil {
		respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}
	if myAccount.ID == "" {
		respondToInteraction(s, i, "Error: Unable to retrieve account information")
		return
	}

	following, err := apiClient.GetFollowing(myAccount.ID)
	if err != nil {
		respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
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
		err = apiClient.FollowAccount(accountInfo.ID)
		if err != nil {
			respondToInteraction(s, i, fmt.Sprintf("Error following account: %v", err))
			return
		}
	}

	// Store the monitored user in the database
	_, err = db.Exec(`
        INSERT OR REPLACE INTO monitored_users 
        (guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at) 
        VALUES (?, ?, ?, ?, '', 0, ?, ?, ?)
    `, i.GuildID, accountInfo.ID, username, channel.ID, mentionRole, avatarLocation, time.Now().Unix())
	if err != nil {
		respondToInteraction(s, i, fmt.Sprintf("Error storing user: %v", err))
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Added %s to monitoring list", username))
}

func handleRemoveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	username := i.ApplicationCommandData().Options[0].StringValue()

	// Remove the monitored user from the database
	result, err := db.Exec(`
        DELETE FROM monitored_users 
        WHERE guild_id = ? AND username = ?
    `, i.GuildID, username)
	if err != nil {
		respondToInteraction(s, i, fmt.Sprintf("Error removing user: %v", err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondToInteraction(s, i, fmt.Sprintf("User %s was not found in the monitoring list", username))
	} else {
		respondToInteraction(s, i, fmt.Sprintf("Removed %s from monitoring list", username))
	}
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

func monitorUsers(s *discordgo.Session) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		err := retryDbOperation(func() error {
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			rows, err := tx.Query("SELECT guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at FROM monitored_users")
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
				}
				err := rows.Scan(&user.GuildID, &user.UserID, &user.Username, &user.NotificationChannel, &user.LastPostID, &user.LastStreamStart, &user.MentionRole, &user.AvatarLocation, &user.AvatarLocationUpdatedAt)
				if err != nil {
					log.Printf("Error scanning row: %v", err)
					continue
				}

				// Check for new posts and live streams
				streamInfo, err := apiClient.GetStreamInfo(user.UserID)
				if err != nil {
					log.Printf("Error fetching stream info: %v", err)
					continue
				}

				// Check if avatar URL needs refreshing (e.g., older than 6 days)
				if time.Now().Unix()-user.AvatarLocationUpdatedAt > 6*24*60*60 {
					newAvatarLocation, err := refreshAvatarURL(user.UserID)
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

					embed := createLiveStreamEmbed(user.Username, streamInfo, user.AvatarLocation)
					mention := "@everyone"
					if user.MentionRole != "" {
						mention = fmt.Sprintf("<@&%s>", user.MentionRole)
					}
					_, err = s.ChannelMessageSendComplex(user.NotificationChannel, &discordgo.MessageSend{
						Content: mention,
						Embed:   embed,
					})
					if err != nil {
						log.Printf("Error sending live stream notification: %v", err)
					}
				}

				postInfo, err := apiClient.GetTimelinePost(user.UserID)
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

					embed := createPostEmbed(user.Username, postInfo[0], user.AvatarLocation)
					mention := "@everyone"
					if user.MentionRole != "" {
						mention = fmt.Sprintf("<@&%s>", user.MentionRole)
					}
					_, err = s.ChannelMessageSendComplex(user.NotificationChannel, &discordgo.MessageSend{
						Content: mention,
						Embed:   embed,
					})
					if err != nil {
						log.Printf("Error sending post notification: %v", err)
					}
				} else if len(postInfo) == 0 {
					log.Printf("No new posts found for user %s", user.Username)
				}
			}

			return tx.Commit()
		})

		if err != nil {
			log.Printf("Error monitoring users: %v", err)
		}
	}
}

func retryDbOperation(operation func() error) error {
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

func createLiveStreamEmbed(username string, streamInfo *api.StreamResponse, avatarLocation string) *discordgo.MessageEmbed {
	liveURL := fmt.Sprintf("https://fansly.com/live/%s", username)
	creatorUrl := fmt.Sprintf("https://fansly.com/%s", username)

	embed := &discordgo.MessageEmbed{
		Title:       "Stream Live!",
		URL:         liveURL,
		Color:       0x03b2f8,
		Description: fmt.Sprintf("%s is now live on Fansly!", username),
		Author: &discordgo.MessageEmbedAuthor{
			URL:     creatorUrl,
			Name:    username,
			IconURL: avatarLocation, // You'll need to fetch the user's avatar URL
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: avatarLocation, // You'll need to fetch the user's avatar URL
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Viewer Count",
				Value:  fmt.Sprintf("%d", streamInfo.Response.Stream.ViewerCount),
				Inline: true,
			},
			{
				Name:   "Started At",
				Value:  time.Unix(streamInfo.Response.Stream.StartedAt/1000, 0).Format(time.RFC1123),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	return embed
}

func createPostEmbed(username string, post api.Post, avatarLocation string) *discordgo.MessageEmbed {
	postURL := fmt.Sprintf("https://fans.ly/post/%s", post.ID)
	creatorUrl := fmt.Sprintf("https://fansly.com/%s", username)
	createdTime := time.Unix(post.CreatedAt, 0)
	//fmt.Printf("CreatedAt: %v, Converted time: %v\n", post.CreatedAt, createdTime)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("New post from %s", username),
		URL:         postURL,
		Color:       0x03b2f8,
		Description: post.Content,
		Author: &discordgo.MessageEmbedAuthor{
			URL:     creatorUrl,
			Name:    username,
			IconURL: avatarLocation, // You'll need to fetch the user's avatar URL
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: avatarLocation,
		},
		Timestamp: createdTime.Format(time.RFC3339),
	}

	// Add media to the embed
	mediaItems, err := apiClient.GetPostMedia(post.ID, apiClient.Token, apiClient.UserAgent)
	//fmt.Printf("Media Items: %v", mediaItems)
	if err != nil {
		log.Printf("Error fetching post media: %v", err)
	} else if len(mediaItems) > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Post Media",
			Value: fmt.Sprintf(":eyes: View on Fansly to see media\n %v", postURL),
		})
	}

	//fmt.Printf("Embed Item: %v", embed)
	return embed
}
