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

// sendPaginatedList now edits the deferred interaction response instead of creating a new one.
func (b *Bot) sendPaginatedList(s *discordgo.Session, i *discordgo.InteractionCreate, items []string, initialPage int) {
	totalPages := int(math.Ceil(float64(len(items)) / float64(itemsPerPage)))

	// Ensure initialPage is valid
	if totalPages == 0 {
		totalPages = 1 // Prevent division by zero or invalid page numbers if items is empty
	}
	if initialPage < 1 {
		initialPage = 1
	}
	if initialPage > totalPages {
		initialPage = totalPages
	}

	// Create initial embed and components
	embed := createPageEmbed(items, initialPage, totalPages)
	components := createPaginationComponents(initialPage, totalPages)

	// Instead of s.InteractionRespond, we use s.InteractionResponseEdit
	// to update the "Thinking..." message that was sent by the deferral.
	// Note that the fields in WebhookEdit are pointers.
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
	if err != nil {
		log.Printf("Error editing interaction response for paginated list: %v", err)
		return
	}

	// Get the message object from the interaction we just edited.
	// This is necessary to get the Message ID for the button collector.
	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Printf("Error getting interaction response message after edit: %v", err)
		return
	}

	// Set up a collector for button interactions on the message we just sent.
	b.setupPaginationCollector(s, i.Member.User.ID, msg.ID, i.ChannelID, items, totalPages)
}

// createPageEmbed creates an embed for a specific page
func createPageEmbed(items []string, page, totalPages int) *discordgo.MessageEmbed {
	startIdx := (page - 1) * itemsPerPage
	endIdx := min(startIdx+itemsPerPage, len(items))

	var pageItems []string
	if startIdx < len(items) {
		pageItems = items[startIdx:endIdx]
	}

	description := "No models are being monitored."
	if len(pageItems) > 0 {
		description = strings.Join(pageItems, "\n\n")
	}

	return &discordgo.MessageEmbed{
		Title:       "Monitored Models",
		Description: description,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Page %d of %d", page, totalPages),
		},
		Color: 0x03b2f8, // A nice blue color
	}
}

// createPaginationComponents creates the button components for pagination
func createPaginationComponents(currentPage, totalPages int) []discordgo.MessageComponent {
	// If there's only one page, no buttons are needed.
	if totalPages <= 1 {
		return nil
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "First",
					Style:    discordgo.SecondaryButton,
					CustomID: "first_page",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏮️"},
					Disabled: currentPage == 1,
				},
				discordgo.Button{
					Label:    "Previous",
					Style:    discordgo.PrimaryButton,
					CustomID: "prev_page",
					Emoji:    &discordgo.ComponentEmoji{Name: "⬅️"},
					Disabled: currentPage == 1,
				},
				discordgo.Button{
					Label:    "Next",
					Style:    discordgo.PrimaryButton,
					CustomID: "next_page",
					Emoji:    &discordgo.ComponentEmoji{Name: "➡️"},
					Disabled: currentPage == totalPages,
				},
				discordgo.Button{
					Label:    "Last",
					Style:    discordgo.SecondaryButton,
					CustomID: "last_page",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏭️"},
					Disabled: currentPage == totalPages,
				},
			},
		},
	}
}

// setupPaginationCollector sets up a collector for pagination button interactions
func (b *Bot) setupPaginationCollector(s *discordgo.Session, userID, messageID, channelID string, items []string, totalPages int) {
	// Create a handler for button interactions
	handlerFunc := s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionMessageComponent || i.Message.ID != messageID {
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
		}

		// Determine the new page based on the button pressed
		newPage := currentPage
		switch i.MessageComponentData().CustomID {
		case "prev_page":
			if currentPage > 1 {
				newPage = currentPage - 1
			}
		case "next_page":
			if currentPage < totalPages {
				newPage = currentPage + 1
			}
		case "first_page":
			newPage = 1
		case "last_page":
			newPage = totalPages
		default:
			// Acknowledge the interaction to prevent it from failing, but do nothing.
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			return
		}

		// If the page hasn't changed, do nothing.
		if newPage == currentPage {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			return
		}

		// Create the new embed and components
		embed := createPageEmbed(items, newPage, totalPages)
		components := createPaginationComponents(newPage, totalPages)

		// Update the message by responding to the button interaction
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: components,
			},
		})
		if err != nil {
			log.Printf("Error updating paginated message: %v", err)
		}
	})

	// Remove the handler and buttons after 5 minutes of inactivity
	time.AfterFunc(5*time.Minute, func() {
		handlerFunc() // This invokes the returned function from AddHandler, which removes it.

		// Get the final state of the embed
		msg, err := s.ChannelMessage(channelID, messageID)
		if err != nil {
			// Message might have been deleted, which is fine.
			return
		}

		// Edit the message to remove the buttons.
		_, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    channelID,
			ID:         messageID,
			Embeds:     &msg.Embeds,                     // Keep the existing embeds
			Components: &[]discordgo.MessageComponent{}, // Set components to an empty slice
		})

		if err != nil {
			log.Printf("Error removing pagination buttons after timeout: %v", err)
		}
	})
}
