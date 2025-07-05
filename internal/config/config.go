package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

var (
	// Bot configuration
	DiscordToken string
	FanslyToken  string
	UserAgent    string
	AppID        string
	PublicKey    string
	LogChannelID string
	BotOwnerID   string

	// Database configuration
	DatabaseType string // "sqlite" or "postgres"
	SqlitePath   string
	PostgresURL  string

	// Application settings
	Debug                       bool
	MonitorIntervalSeconds      int
	StatusUpdateIntervalMinutes int
	AvatarRefreshIntervalHours  int
	MonitorWorkerCount          int
	MaxMonitoredUsersPerGuild   int

	ApiRequestsPerSecond float64
	ApiBurst             int
)

func Load() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file, falling back to environment variables")
	}

	// Bot configuration
	DiscordToken = os.Getenv("DISCORD_TOKEN")
	FanslyToken = os.Getenv("FANSLY_TOKEN")
	UserAgent = os.Getenv("USER_AGENT")
	AppID = os.Getenv("APP_ID")
	PublicKey = os.Getenv("PUBLIC_KEY")
	LogChannelID = os.Getenv("LOG_CHANNEL_ID")
	BotOwnerID = os.Getenv("BOT_OWNER_ID")

	if DiscordToken == "" || FanslyToken == "" || UserAgent == "" || AppID == "" || PublicKey == "" {
		log.Fatal("Missing required environment variables")
	}

	// Database configuration
	DatabaseType = os.Getenv("DB_TYPE")
	if DatabaseType == "" {
		DatabaseType = "sqlite" // Default to SQLite
	}

	SqlitePath = os.Getenv("SQLITE_PATH")
	if SqlitePath == "" && DatabaseType == "sqlite" {
		SqlitePath = "bot.db" // Default path
	}

	PostgresURL = os.Getenv("POSTGRES_URL")
	if PostgresURL == "" && DatabaseType == "postgres" {
		log.Fatal("POSTGRES_URL environment variable required when using postgres")
	}

	// Application settings
	debugStr := os.Getenv("DEBUG")
	Debug, _ = strconv.ParseBool(debugStr)

	MonitorIntervalSeconds = getEnvAsInt("MONITOR_INTERVAL_SECONDS", 120)            // Default: 2 minutes
	StatusUpdateIntervalMinutes = getEnvAsInt("STATUS_UPDATE_INTERVAL_MINUTES", 120) // Default: 2 hours
	AvatarRefreshIntervalHours = getEnvAsInt("AVATAR_REFRESH_INTERVAL_HOURS", 144)   // Default: 6 days (6 * 24)
	MonitorWorkerCount = getEnvAsInt("MONITOR_WORKER_COUNT", 10)                     // Default: 10 workers
	MaxMonitoredUsersPerGuild = getEnvAsInt("MAX_MONITORED_USERS_PER_GUILD", 5)

	ApiRequestsPerSecond = getEnvAsFloat64("API_REQUESTS_PER_SECOND", 2.0)
	ApiBurst = getEnvAsInt("API_BURST", 5)
}

func getEnvAsFloat64(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

// GetDatabaseConnectionString returns the database connection string based on database type
func GetDatabaseConnectionString() string {
	switch DatabaseType {
	case "postgres":
		return PostgresURL
	case "sqlite":
		return SqlitePath
	default:
		return SqlitePath
	}
}
