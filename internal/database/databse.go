package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fvckgrimm/discord-fansly-notify/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const currentVersion = 3

var (
	DB     *gorm.DB
	SqlDB  *sql.DB // For backward compatibility with existing code
	DBType string  // "sqlite" or "postgres"
)

// Init initializes the database connection
func Init(dbType string, connString string) error {
	var err error
	DBType = dbType

	// Set up GORM logger
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	// Connect to the database
	gormConfig := &gorm.Config{
		Logger: gormLogger,
	}

	// Initialize DB based on type
	switch dbType {
	case "sqlite":
		DB, err = gorm.Open(sqlite.Open(connString), gormConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to SQLite database: %w", err)
		}
		// Configure SQLite for better concurrent access
		sqlDB, err := DB.DB()
		if err != nil {
			return fmt.Errorf("failed to get DB instance: %w", err)
		}
		// Set connection pool settings
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
		// Enable WAL mode for better concurrency
		DB.Exec("PRAGMA journal_mode = WAL")
		DB.Exec("PRAGMA busy_timeout = 5000")

	case "postgres":
		DB, err = gorm.Open(postgres.Open(connString), gormConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return fmt.Errorf("failed to get DB instance: %w", err)
		}
		// Set connection pool settings
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)

	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	// Store SQL DB for backward compatibility
	SqlDB, err = DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	// Auto-migrate models
	err = DB.AutoMigrate(&models.SchemaVersion{}, &models.MonitoredUser{})
	if err != nil {
		return fmt.Errorf("failed to migrate database schema: %w", err)
	}

	// Get current schema version
	var version models.SchemaVersion
	result := DB.First(&version)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// No version found, assume fresh install
			DB.Create(&models.SchemaVersion{Version: 0})
			version.Version = 0
		} else {
			return fmt.Errorf("failed to get schema version: %w", result.Error)
		}
	}

	// Run migrations if needed
	err = runMigrations(version.Version)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Printf("Connected to %s database successfully", dbType)
	return nil
}

// Close closes the database connection
func Close() {
	if SqlDB != nil {
		SqlDB.Close()
	}
}

// runMigrations runs database migrations
func runMigrations(currentDBVersion int) error {
	migrations := []func(*gorm.DB) error{
		migrateToV1,
		migrateToV2,
		migrateToV3,
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
			return fmt.Errorf("migration to version %d failed: %v", version, err)
		}

		// Update schema version
		DB.Model(&models.SchemaVersion{}).Where("1=1").Update("version", version)
		log.Printf("Migration to version %d completed", version)
	}

	return nil
}

// migrateToV1 creates the initial schema
func migrateToV1(db *gorm.DB) error {
	// This is handled by AutoMigrate now, but we keep the function for versioning
	return nil
}

func migrateToV2(db *gorm.DB) error {
	// The columns are already defined in the model
	// This is just here for backward compatibility
	return nil
}

func migrateToV3(db *gorm.DB) error {
	// The columns are already defined in the model
	// This is just here for backward compatibility
	return nil
}

// WithRetry performs a database operation with retry logic for locked database
func WithRetry(operation func() error) error {
	maxRetries := 5
	retries := make([]int, maxRetries)

	for i := range retries {
		err := operation()
		if err == nil {
			return nil
		}

		// Special handling for SQLite busy errors
		if DBType == "sqlite" && (err.Error() == "database is locked" || err.Error() == "database is busy") {
			backoff := time.Duration(100*(i+1)) * time.Millisecond
			log.Printf("Database locked, retrying in %v (attempt %d/%d)", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			continue
		}

		return err
	}
	return fmt.Errorf("operation failed after %d retries", maxRetries)
}
