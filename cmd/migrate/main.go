package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// This file needs to import the Migration/MigrationManager from main package
// For a real project, you would move migration code to a separate package

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

// Forward declaration of MigrationManager functions
func NewMigrationManager(db *sql.DB) *MigrationManager {
	// This is just a stub for compilation, the real logic lives in migrations.go
	return &MigrationManager{db: db, migrations: []*Migration{}}
}

func (mm *MigrationManager) LoadMigrationsFromDir(dir string) error { return nil }

func main() {
	// Define commands
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	createDesc := createCmd.String("desc", "", "Migration description")

	// Parse command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Expected 'create' subcommand")
		os.Exit(1)
	}

	// Handle different commands
	switch os.Args[1] {
	case "create":
		createCmd.Parse(os.Args[2:])
		if *createDesc == "" {
			fmt.Println("Please provide a migration description with -desc")
			os.Exit(1)
		}

		// Create a temporary DB connection for the migration manager
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			log.Fatalf("Failed to open temporary database: %v", err)
		}
		defer db.Close()

		// Create migration manager
		mm := NewMigrationManager(db)

		// Load existing migrations to determine next version
		migrationsDir := filepath.Join("..", "migrations")
		if err := mm.LoadMigrationsFromDir(migrationsDir); err != nil {
			log.Fatalf("Failed to load migrations: %v", err)
		}

		// Create the migration file
		filename, err := mm.CreateMigrationFile(migrationsDir, *createDesc)
		if err != nil {
			log.Fatalf("Failed to create migration file: %v", err)
		}

		fmt.Printf("Created migration file: %s\n", filename)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// This is a stub implementation since the real implementation is in migrations.go
func (mm *MigrationManager) CreateMigrationFile(dir, description string) (string, error) {
	// Create migrations directory if it doesn't exist
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Find all existing migration files to determine next version
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Find highest version number
	highestVersion := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasPrefix(name, "V") && strings.Contains(name, "__") {
			versionStr := strings.Split(name, "__")[0][1:]
			var version int
			fmt.Sscanf(versionStr, "%d", &version)
			if version > highestVersion {
				highestVersion = version
			}
		}
	}

	// Next version number
	nextVersion := highestVersion + 1

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
