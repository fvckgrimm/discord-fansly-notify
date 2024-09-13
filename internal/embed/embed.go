package embed

import (
	"fmt"
	//"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fvckgrimm/discord-fansly-notify/api"
)

func CreateLiveStreamEmbed(username string, streamInfo *api.StreamResponse, avatarLocation string, liveImageURL string) *discordgo.MessageEmbed {
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
			IconURL: avatarLocation,
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: avatarLocation,
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

	if liveImageURL != "" {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: liveImageURL,
		}
	}

	return embed
}

func CreatePostEmbed(username string, post api.Post, avatarLocation string, postMedia []api.AccountMedia) *discordgo.MessageEmbed {
	postURL := fmt.Sprintf("https://fans.ly/post/%s", post.ID)
	creatorUrl := fmt.Sprintf("https://fansly.com/%s", username)
	createdTime := time.Unix(post.CreatedAt, 0)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("New post from %s", username),
		URL:         postURL,
		Color:       0x03b2f8,
		Description: post.Content,
		Author: &discordgo.MessageEmbedAuthor{
			URL:     creatorUrl,
			Name:    username,
			IconURL: avatarLocation,
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: avatarLocation,
		},
		Timestamp: createdTime.Format(time.RFC3339),
	}

	// Add media to the embed
	if len(postMedia) > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Post Media",
			Value: fmt.Sprintf(":eyes: View on Fansly to see media\n %v", postURL),
		})
	}

	return embed
}
