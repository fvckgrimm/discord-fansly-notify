package database

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

const currentVersion = 2

func Init() {
	var err error
	DB, err = sql.Open("sqlite", "bot.db")
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS schema_version (
            version INTEGER PRIMARY KEY
        )
    `)
	if err != nil {
		log.Fatal(err)
	}

	// Get current schema version
	var version int
	err = DB.QueryRow("SELECT version FROM schema_version").Scan(&version)
	if err != nil {
		// No version found, assume fresh install
		_, err = DB.Exec("INSERT INTO schema_version (version) VALUES (0)")
		if err != nil {
			log.Fatal(err)
		}
		version = 0
	}

	// Run migrations
	runMigrations(version)
}

func runMigrations(currentDBVersion int) {
	migrations := []func(*sql.DB) error{
		migrateToV1,
		migrateToV2,
		// Add new migrations here
	}

	for i, migration := range migrations {
		version := i + 1
		if version <= currentDBVersion {
			continue
		}

		log.Printf("Running migration to version %d", version)
		err := migration(DB)
		if err != nil {
			log.Fatalf("Migration to version %d failed: %v", version, err)
		}

		_, err = DB.Exec("UPDATE schema_version SET version = ?", version)
		if err != nil {
			log.Fatalf("Failed to update schema version: %v", err)
		}
		log.Printf("Migration to version %d completed", version)
	}
}

func migrateToV1(db *sql.DB) error {
	// Initial schema
	_, err := db.Exec(`
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
            live_image_url TEXT,
            posts_enabled BOOLEAN DEFAULT 1,
            live_enabled BOOLEAN DEFAULT 1,
            PRIMARY KEY (guild_id, user_id)
        )
    `)
	return err
}

func migrateToV2(db *sql.DB) error {
	// Add separate notification channels
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Add new columns
	_, err = tx.Exec(`
        ALTER TABLE monitored_users 
        ADD COLUMN post_notification_channel TEXT;
    `)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
        ALTER TABLE monitored_users 
        ADD COLUMN live_notification_channel TEXT;
    `)
	if err != nil {
		return err
	}

	// Set default values from existing notification_channel
	_, err = tx.Exec(`
        UPDATE monitored_users 
        SET post_notification_channel = notification_channel,
            live_notification_channel = notification_channel
    `)
	if err != nil {
		return err
	}

	return tx.Commit()
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
			post_notification_channel TEXT,
            live_notification_channel TEXT,
			last_post_id TEXT,
			last_stream_start INTEGER,
			mention_role TEXT,
			avatar_location TEXT,
			avatar_location_updated_at INTEGER,
			live_image_url	TEXT,
			posts_enabled BOOLEAN DEFAULT 1,
            live_enabled BOOLEAN DEFAULT 1,
			PRIMARY KEY (guild_id, user_id)
		)
	`)
	if err != nil {
		log.Fatal(err)
	}
}
