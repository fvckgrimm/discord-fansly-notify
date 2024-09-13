package database

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init() {
	var err error
	DB, err = sql.Open("sqlite", "bot.db")
	if err != nil {
		log.Fatal(err)
	}

	createTables()
}

func Close() {
	DB.Close()
}

func createTables() {
	_, err := DB.Exec(`
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
			live_image_url	TEXT,
			PRIMARY KEY (guild_id, user_id)
		)
	`)
	if err != nil {
		log.Fatal(err)
	}
}
