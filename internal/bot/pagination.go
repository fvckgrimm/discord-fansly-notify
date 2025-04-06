package bot

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	itemsPerPage = 5
)

// sendPaginatedList sends a paginated list of monitored users
func (b *Bot) sendPaginatedList(s *discordgo.Session, i *discordgo.InteractionCreate, items []string, initialPage int) {
	totalPages := int(math.Ceil(float64(len(items)) / float64(itemsPerPage)))

	// Ensure initialPage is valid
	if initialPage < 1 {
		initialPage = 1
	}
	if initialPage > totalPages {
		initialPage = totalPages
	}

	// Create initial embed for the requested page
	embed := createPageEmbed(items, initialPage, totalPages)

	// Create navigation components
	components := createPaginationComponents(initialPage, totalPages)

	// Respond with the initial message
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})

	if err != nil {
		log.Printf("Error sending paginated list: %v", err)
		return
	}

	// Get the message ID for the response
	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Printf("Error getting interaction response: %v", err)
		return
	}

	// Set up a collector for button interactions
	b.setupPaginationCollector(s, i.Member.User.ID, msg.ID, i.ChannelID, items, totalPages)
}

// createPageEmbed creates an embed for a specific page
func createPageEmbed(items []string, page, totalPages int) *discordgo.MessageEmbed {
	startIdx := (page - 1) * itemsPerPage
	//endIdx := startIdx + itemsPerPage
	endIdx := min(startIdx+itemsPerPage, len(items))

	pageItems := items[startIdx:endIdx]

	return &discordgo.MessageEmbed{
		Title:       "Monitored Models",
		Description: strings.Join(pageItems, "\n\n"),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Page %d of %d", page, totalPages),
		},
		Color: 0x03b2f8,
	}
}

// createPaginationComponents creates the button components for pagination
func createPaginationComponents(currentPage, totalPages int) []discordgo.MessageComponent {
	// First button row with navigation
	var buttons []discordgo.MessageComponent

	// Previous button
	prevButton := discordgo.Button{
		Label:    "Previous",
		Style:    discordgo.PrimaryButton,
		CustomID: "prev_page",
		Emoji: &discordgo.ComponentEmoji{
			Name: "⬅️",
		},
		Disabled: currentPage <= 1,
	}

	// Next button
	nextButton := discordgo.Button{
		Label:    "Next",
		Style:    discordgo.PrimaryButton,
		CustomID: "next_page",
		Emoji: &discordgo.ComponentEmoji{
			Name: "➡️",
		},
		Disabled: currentPage >= totalPages,
	}

	// First page button
	firstButton := discordgo.Button{
		Label:    "First",
		Style:    discordgo.SecondaryButton,
		CustomID: "first_page",
		Emoji: &discordgo.ComponentEmoji{
			Name: "⏮️",
		},
		Disabled: currentPage <= 1,
	}

	// Last page button
	lastButton := discordgo.Button{
		Label:    "Last",
		Style:    discordgo.SecondaryButton,
		CustomID: "last_page",
		Emoji: &discordgo.ComponentEmoji{
			Name: "⏭️",
		},
		Disabled: currentPage >= totalPages,
	}

	buttons = append(buttons, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			firstButton,
			prevButton,
			nextButton,
			lastButton,
		},
	})

	return buttons
}

// setupPaginationCollector sets up a collector for pagination button interactions
func (b *Bot) setupPaginationCollector(s *discordgo.Session, userID, messageID, channelID string, items []string, totalPages int) {
	// Create a handler for button interactions
	handlerFunc := s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Only process button interactions for this message
		if i.Message == nil || i.Message.ID != messageID {
			return
		}

		// Only allow the original command user to use the buttons
		if i.Member.User.ID != userID {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Only the user who ran the command can use these buttons.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Get the current page from the footer
		var currentPage int
		if len(i.Message.Embeds) > 0 && i.Message.Embeds[0].Footer != nil {
			fmt.Sscanf(i.Message.Embeds[0].Footer.Text, "Page %d of %d", &currentPage, &totalPages)
		} else {
			currentPage = 1
		}

		// Determine the new page based on the button pressed
		var newPage int
		switch i.MessageComponentData().CustomID {
		case "prev_page":
			newPage = currentPage - 1
			newPage = max(1, newPage)
		case "next_page":
			newPage = currentPage + 1
			newPage = min(newPage, totalPages)
		case "first_page":
			newPage = 1
		case "last_page":
			newPage = totalPages
		default:
			return
		}

		// Create the new embed and components
		embed := createPageEmbed(items, newPage, totalPages)
		components := createPaginationComponents(newPage, totalPages)

		// Update the message
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: components,
			},
		})
	})

	// Remove the handler after 5 minutes
	time.AfterFunc(5*time.Minute, func() {
		// Call the handler function to remove it
		handlerFunc()

		// Update the message to remove buttons after timeout
		embed := createPageEmbed(items, 1, totalPages)

		// Use ChannelMessageEditEmbed instead of ChannelMessageEditComplex
		_, err := s.ChannelMessageEditEmbed(channelID, messageID, embed)
		if err != nil {
			log.Printf("Error removing pagination embed: %v", err)
		}

		// Remove components in a separate call - fix the pointer issue
		emptyComponents := []discordgo.MessageComponent{}
		_, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    channelID,
			ID:         messageID,
			Components: &emptyComponents,
		})

		if err != nil {
			log.Printf("Error removing pagination buttons: %v", err)
		}
	})
}
