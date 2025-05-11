package database

import (
	"errors"
	"github.com/fvckgrimm/discord-fansly-notify/internal/models"
	//"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository handles database operations for monitored users
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new repository instance
func NewRepository() *Repository {
	return &Repository{db: DB}
}

// GetMonitoredUsers returns all monitored users
func (r *Repository) GetMonitoredUsers() ([]models.MonitoredUser, error) {
	var users []models.MonitoredUser
	err := WithRetry(func() error {
		return r.db.Find(&users).Error
	})
	return users, err
}

// GetMonitoredUser returns a specific monitored user
func (r *Repository) GetMonitoredUser(guildID, userID string) (*models.MonitoredUser, error) {
	var user models.MonitoredUser
	err := WithRetry(func() error {
		result := r.db.Where("guild_id = ? AND user_id = ?", guildID, userID).First(&user)
		return result.Error
	})

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // No user found
	}

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) GetMonitoredUserByUsername(guildID, username string) (*models.MonitoredUser, error) {
	var user models.MonitoredUser
	result := r.db.Where("guild_id = ? AND username = ?", guildID, username).First(&user)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil // No user found
	}

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}

// AddMonitoredUser adds a new monitored user
func (r *Repository) AddMonitoredUser(user *models.MonitoredUser) error {
	return WithRetry(func() error {
		return r.db.Create(user).Error
	})
}

func (r *Repository) AddOrUpdateMonitoredUser(user *models.MonitoredUser) error {
	return WithRetry(func() error {
		return r.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "guild_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"username", "notification_channel", "post_notification_channel", "live_notification_channel",
				"last_post_id", "last_stream_start", "mention_role", "avatar_location",
				"avatar_location_updated_at", "live_image_url", "posts_enabled", "live_enabled",
				"live_mention_role", "post_mention_role",
			}),
		}).Create(user).Error
	})
}

// UpdateMonitoredUser updates an existing monitored user
func (r *Repository) UpdateMonitoredUser(user *models.MonitoredUser) error {
	return WithRetry(func() error {
		return r.db.Save(user).Error
	})
}

// DeleteMonitoredUser deletes a monitored user
func (r *Repository) DeleteMonitoredUser(guildID, userID string) error {
	return WithRetry(func() error {
		return r.db.Delete(&models.MonitoredUser{}, "guild_id = ? AND user_id = ?", guildID, userID).Error
	})
}

func (r *Repository) DeleteMonitoredUserByUsername(guildID, username string) error {
	result := r.db.Delete(&models.MonitoredUser{}, "guild_id = ? AND username = ?", guildID, username)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// GetMonitoredUsersForGuild returns all monitored users for a specific guild
func (r *Repository) GetMonitoredUsersForGuild(guildID string) ([]models.MonitoredUser, error) {
	var users []models.MonitoredUser
	err := WithRetry(func() error {
		return r.db.Where("guild_id = ?", guildID).Find(&users).Error
	})
	return users, err
}

// UpdateLastPostID updates the last post ID for a monitored user
func (r *Repository) UpdateLastPostID(guildID, userID, postID string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("last_post_id", postID).Error
	})
}

func (r *Repository) UpdateLastPostIDByUsername(guildID, username, postID string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("last_post_id", postID)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdateLastStreamStart updates the last stream start for a monitored user
func (r *Repository) UpdateLastStreamStart(guildID, userID string, timestamp int64) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("last_stream_start", timestamp).Error
	})
}

// UpdateAvatarInfo updates the avatar information for a monitored user
func (r *Repository) UpdateAvatarInfo(guildID, userID, avatarLocation string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Updates(map[string]any{
				"avatar_location":            avatarLocation,
				"avatar_location_updated_at": time.Now().Unix(),
			}).Error
	})
}

func (r *Repository) UpdateAvatarInfoByUsername(guildID, username, avatarLocation string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Updates(map[string]any{
			"avatar_location":            avatarLocation,
			"avatar_location_updated_at": time.Now().Unix(),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// DisablePosts disables post notifications for a monitored user
func (r *Repository) DisablePosts(guildID, userID string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("posts_enabled", false).Error
	})
}

// EnablePosts enables post notifications for a monitored user
func (r *Repository) EnablePosts(guildID, userID string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("posts_enabled", true).Error
	})
}

// DisableLive disables live stream notifications for a monitored user
func (r *Repository) DisableLive(guildID, userID string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("live_enabled", false).Error
	})
}

// EnableLive enables live stream notifications for a monitored user
func (r *Repository) EnableLive(guildID, userID string) error {
	return WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("live_enabled", true).Error
	})
}

// CountMonitoredUsers counts all monitored users
func (r *Repository) CountMonitoredUsers() (int64, error) {
	var count int64
	err := WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).Count(&count).Error
	})
	return count, err
}

// CountGuilds counts unique guilds with monitored users
func (r *Repository) CountGuilds() (int64, error) {
	var count int64
	err := WithRetry(func() error {
		return r.db.Model(&models.MonitoredUser{}).Distinct("guild_id").Count(&count).Error
	})
	return count, err
}

// DeleteAllUsersInGuild deletes all monitored users for a specific guild
func (r *Repository) DeleteAllUsersInGuild(guildID string) error {
	return WithRetry(func() error {
		return r.db.Delete(&models.MonitoredUser{}, "guild_id = ?", guildID).Error
	})
}

// UpdateLiveImageURL updates the live image URL for a monitored user
func (r *Repository) UpdateLiveImageURL(guildID, username, imageURL string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("live_image_url", imageURL)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdatePostChannel updates the post notification channel for a monitored user
func (r *Repository) UpdatePostChannel(guildID, username, channelID string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("post_notification_channel", channelID)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdateLiveChannel updates the live notification channel for a monitored user
func (r *Repository) UpdateLiveChannel(guildID, username, channelID string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("live_notification_channel", channelID)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdatePostMentionRole updates the post mention role for a monitored user
func (r *Repository) UpdatePostMentionRole(guildID, username, roleID string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("post_mention_role", roleID)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdateLiveMentionRole updates the live mention role for a monitored user
func (r *Repository) UpdateLiveMentionRole(guildID, username, roleID string) error {
	result := r.db.Model(&models.MonitoredUser{}).
		Where("guild_id = ? AND username = ?", guildID, username).
		Update("live_mention_role", roleID)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}
