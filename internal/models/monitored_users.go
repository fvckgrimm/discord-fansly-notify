package models

type MonitoredUser struct {
	GuildID                 string `gorm:"primaryKey;column:guild_id"`
	UserID                  string `gorm:"primaryKey;column:user_id"`
	Username                string `gorm:"column:username"`
	NotificationChannel     string `gorm:"column:notification_channel"`
	PostNotificationChannel string `gorm:"column:post_notification_channel"`
	LiveNotificationChannel string `gorm:"column:live_notification_channel"`
	LastPostID              string `gorm:"column:last_post_id"`
	LastStreamStart         int64  `gorm:"column:last_stream_start"`
	MentionRole             string `gorm:"column:mention_role"`
	AvatarLocation          string `gorm:"column:avatar_location"`
	AvatarLocationUpdatedAt int64  `gorm:"column:avatar_location_updated_at"`
	LiveImageURL            string `gorm:"column:live_image_url"`
	PostsEnabled            bool   `gorm:"default:true"`
	LiveEnabled             bool   `gorm:"default:true"`
	LiveMentionRole         string `gorm:"column:live_mention_role"`
	PostMentionRole         string `gorm:"column:post_mention_role"`
}

type SchemaVersion struct {
	Version int `gorm:"primaryKey"`
}

func (MonitoredUser) TableName() string {
	return "monitored_users"
}
