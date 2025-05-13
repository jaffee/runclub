package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db         *sql.DB
	migrations []*Migration
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *sql.DB) *MigrationManager {
	return &MigrationManager{
		db:         db,
		migrations: make([]*Migration, 0),
	}
}

// LoadMigrationsFromDir loads all SQL migrations from a directory
func (mm *MigrationManager) LoadMigrationsFromDir(dir string) error {
	// Create migrations directory if it doesn't exist
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Define file pattern for migrations: V1__description.sql
	filePattern := regexp.MustCompile(`^V(\d+)__(.+)\.sql$`)

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Process each migration file
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		matches := filePattern.FindStringSubmatch(file.Name())
		if matches == nil || len(matches) != 3 {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return fmt.Errorf("invalid migration version in file %s: %w", file.Name(), err)
		}

		description := strings.ReplaceAll(matches[2], "_", " ")

		// Read the migration SQL
		sql, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
		}

		// Add the migration
		mm.migrations = append(mm.migrations, &Migration{
			Version:     version,
			Description: description,
			SQL:         string(sql),
		})
	}

	// Sort migrations by version
	sort.Slice(mm.migrations, func(i, j int) bool {
		return mm.migrations[i].Version < mm.migrations[j].Version
	})

	return nil
}

// InitMigrationTable ensures the migrations table exists
func (mm *MigrationManager) InitMigrationTable() error {
	_, err := mm.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// GetAppliedMigrations gets the list of already applied migrations
func (mm *MigrationManager) GetAppliedMigrations() (map[int]bool, error) {
	rows, err := mm.db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions[version] = true
	}

	return versions, rows.Err()
}

// Migrate runs all pending migrations
func (mm *MigrationManager) Migrate() error {
	// Init migration table
	if err := mm.InitMigrationTable(); err != nil {
		return fmt.Errorf("failed to init migrations table: %w", err)
	}

	// Get applied migrations
	appliedMigrations, err := mm.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Run each pending migration in a transaction
	for _, migration := range mm.migrations {
		if appliedMigrations[migration.Version] {
			log.Printf("Migration %d already applied, skipping", migration.Version)
			continue
		}

		log.Printf("Applying migration %d: %s", migration.Version, migration.Description)

		// Start transaction
		tx, err := mm.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		// Execute migration SQL
		_, err = tx.Exec(migration.SQL)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		// Record migration
		_, err = tx.Exec(
			"INSERT INTO schema_migrations (version, description) VALUES (?, ?)",
			migration.Version, migration.Description,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}

		log.Printf("Migration %d applied successfully", migration.Version)
	}

	return nil
}

// CreateMigrationFile creates a new migration file with the given description
func (mm *MigrationManager) CreateMigrationFile(dir, description string) (string, error) {
	// Create migrations directory if it doesn't exist
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Get the next version number
	nextVersion := 1
	if len(mm.migrations) > 0 {
		nextVersion = mm.migrations[len(mm.migrations)-1].Version + 1
	}

	// Format description for filename
	fileDescription := strings.ReplaceAll(description, " ", "_")
	filename := fmt.Sprintf("V%d__%s.sql", nextVersion, fileDescription)
	filepath := filepath.Join(dir, filename)

	// Create empty migration file
	err = os.WriteFile(filepath, []byte("-- Migration: "+description+"\n\n"), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create migration file: %w", err)
	}

	return filepath, nil
}