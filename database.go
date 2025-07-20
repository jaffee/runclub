package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Database represents our SQLite database
type Database struct {
	db    *sql.DB
	mutex sync.RWMutex
}

// NewDatabase creates a new SQLite database
func NewDatabase() (*Database, error) {
	// Create database file if it doesn't exist
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "/data/runclub.db"
		// Use local path if data directory doesn't exist (for local development)
		if _, err := os.Stat("/data"); os.IsNotExist(err) {
			dbPath = "runclub.db"
		}
	}

	// Check if we need to initialize the database
	isNew := false
	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		isNew = true
	}

	// Open the database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to wait up to 5 seconds when database is locked
	_, err = db.Exec("PRAGMA busy_timeout=5000")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Set connection parameters
	// Allow multiple connections for better concurrency with WAL mode
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Create a new database instance
	database := &Database{
		db: db,
	}

	// Initialize schema if needed
	if isNew {
		if err := database.initSchema(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize database schema: %w", err)
		}
	}

	// Run migrations
	if err := database.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return database, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	return db.db.Close()
}

// runMigrations applies any pending database migrations
func (db *Database) runMigrations() error {
	// Create migration manager
	mm := NewMigrationManager(db.db)

	// Load migrations from directory
	if err := mm.LoadMigrationsFromDir("migrations"); err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Apply migrations
	if err := mm.Migrate(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// initSchema creates the database tables
func (db *Database) initSchema() error {
	// Read the schema SQL file
	schemaSQL, err := os.ReadFile("db_schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Execute the schema SQL
	_, err = db.db.Exec(string(schemaSQL))
	if err != nil {
		return fmt.Errorf("failed to execute schema SQL: %w", err)
	}

	return nil
}

// SaveSeason saves a season to the database
func (db *Database) SaveSeason(season *Season) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// Begin a transaction
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// If this season is active, deactivate all other seasons first
	if season.IsActive {
		_, err = tx.Exec("UPDATE seasons SET is_active = 0 WHERE is_active = 1")
		if err != nil {
			return fmt.Errorf("failed to deactivate other seasons: %w", err)
		}
	}

	// Generate registration token if not provided
	if season.RegistrationToken == "" {
		season.RegistrationToken = uuid.New().String()
	}

	// Insert the new season
	_, err = tx.Exec(
		`INSERT INTO seasons (id, name, is_active, created_at, registration_token) 
		VALUES (?, ?, ?, ?, ?)`,
		season.ID, season.Name, season.IsActive, season.CreatedAt, season.RegistrationToken,
	)
	if err != nil {
		return fmt.Errorf("failed to save season: %w", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetSeason retrieves a season by ID
func (db *Database) GetSeason(id string) (*Season, bool, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	season := &Season{}
	var registrationToken sql.NullString
	err := db.db.QueryRow(
		`SELECT id, name, is_active, created_at, registration_token FROM seasons WHERE id = ?`,
		id,
	).Scan(&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &registrationToken)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("failed to get season: %w", err)
	}

	if registrationToken.Valid {
		season.RegistrationToken = registrationToken.String
	}

	return season, true, nil
}

// GetSeasonByRegistrationToken retrieves a season by its registration token
func (db *Database) GetSeasonByRegistrationToken(token string) (*Season, bool, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	season := &Season{}
	var registrationToken sql.NullString
	err := db.db.QueryRow(
		`SELECT id, name, is_active, created_at, registration_token FROM seasons WHERE registration_token = ?`,
		token,
	).Scan(&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &registrationToken)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("failed to get season by token: %w", err)
	}

	if registrationToken.Valid {
		season.RegistrationToken = registrationToken.String
	}

	return season, true, nil
}

// GetActiveSeason retrieves the currently active season
func (db *Database) GetActiveSeason() (*Season, bool, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	season := &Season{}
	var registrationToken sql.NullString
	err := db.db.QueryRow(
		`SELECT id, name, is_active, created_at, registration_token FROM seasons WHERE is_active = 1`,
	).Scan(&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &registrationToken)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("failed to get active season: %w", err)
	}

	if registrationToken.Valid {
		season.RegistrationToken = registrationToken.String
	}

	return season, true, nil
}

// SetActiveSeason sets a season as active and deactivates all others
func (db *Database) SetActiveSeason(seasonID string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// Begin a transaction
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Deactivate all seasons
	_, err = tx.Exec("UPDATE seasons SET is_active = 0")
	if err != nil {
		return fmt.Errorf("failed to deactivate seasons: %w", err)
	}

	// Activate the specified season
	result, err := tx.Exec("UPDATE seasons SET is_active = 1 WHERE id = ?", seasonID)
	if err != nil {
		return fmt.Errorf("failed to activate season: %w", err)
	}

	// Check if the season exists
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("season not found: %s", seasonID)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetAllSeasons returns all seasons
func (db *Database) GetAllSeasons() ([]*Season, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	rows, err := db.db.Query(
		`SELECT id, name, is_active, created_at, registration_token 
		FROM seasons
		ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query seasons: %w", err)
	}
	defer rows.Close()

	var seasons []*Season
	for rows.Next() {
		season := &Season{}
		var registrationToken sql.NullString
		err := rows.Scan(
			&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &registrationToken,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan season row: %w", err)
		}
		if registrationToken.Valid {
			season.RegistrationToken = registrationToken.String
		}
		seasons = append(seasons, season)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating season rows: %w", err)
	}

	return seasons, nil
}

// SaveRegistration saves a registration to the database
func (db *Database) SaveRegistration(reg *Registration) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, err := db.db.Exec(
		`INSERT INTO registrations (
			id, season_id, first_name, last_name, grade, teacher, gender,
			parent_contact_number, backup_contact_number, parent_email, 
			dismissal_method, allergies, medical_info, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reg.ID, reg.SeasonID, reg.FirstName, reg.LastName, reg.Grade, reg.Teacher, reg.Gender,
		reg.ParentContactNumber, reg.BackupContactNumber, reg.ParentEmail,
		reg.DismissalMethod, reg.Allergies, reg.MedicalInfo, reg.RegisteredAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save registration: %w", err)
	}

	return nil
}

// GetRegistration retrieves a registration by ID
func (db *Database) GetRegistration(id string) (*Registration, bool, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	reg := &Registration{}
	var seasonID sql.NullString
	var genderNull sql.NullString
	var dismissalMethodNull sql.NullString
	var allergiesNull sql.NullString
	var medicalInfoNull sql.NullString

	// Query registration with season data
	err := db.db.QueryRow(
		`SELECT
			r.id, r.season_id, r.first_name, r.last_name, r.grade, r.teacher, r.gender,
			r.parent_contact_number, r.backup_contact_number, r.parent_email, 
			r.dismissal_method, r.allergies, r.medical_info, r.registered_at
		FROM registrations r WHERE r.id = ?`,
		id,
	).Scan(
		&reg.ID, &seasonID, &reg.FirstName, &reg.LastName, &reg.Grade, &reg.Teacher, &genderNull,
		&reg.ParentContactNumber, &reg.BackupContactNumber, &reg.ParentEmail,
		&dismissalMethodNull, &allergiesNull, &medicalInfoNull, &reg.RegisteredAt,
	)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("failed to get registration: %w", err)
	}

	// Handle NULL values
	if genderNull.Valid {
		reg.Gender = genderNull.String
	} else {
		reg.Gender = "" // Use empty string for NULL gender
	}

	if dismissalMethodNull.Valid {
		reg.DismissalMethod = dismissalMethodNull.String
	} else {
		reg.DismissalMethod = ""
	}

	if allergiesNull.Valid {
		reg.Allergies = allergiesNull.String
	} else {
		reg.Allergies = ""
	}

	if medicalInfoNull.Valid {
		reg.MedicalInfo = medicalInfoNull.String
	} else {
		reg.MedicalInfo = ""
	}

	// Set season ID if not null
	if seasonID.Valid {
		reg.SeasonID = &seasonID.String

		// Fetch the associated season
		season := &Season{}
		var seasonRegToken sql.NullString
		err = db.db.QueryRow(
			`SELECT id, name, is_active, created_at, registration_token
			FROM seasons WHERE id = ?`,
			reg.SeasonID,
		).Scan(&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &seasonRegToken)

		if err != nil && err != sql.ErrNoRows {
			return nil, false, fmt.Errorf("failed to get season for registration: %w", err)
		}

		if err == nil {
			if seasonRegToken.Valid {
				season.RegistrationToken = seasonRegToken.String
			}
			reg.Season = season
		}
	}

	return reg, true, nil
}

// GetAllRegistrations returns all registrations, optionally filtered by season
func (db *Database) GetAllRegistrations(seasonID string) ([]*Registration, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	query := `SELECT
		r.id, r.season_id, r.first_name, r.last_name, r.grade, r.teacher, r.gender,
		r.parent_contact_number, r.backup_contact_number, r.parent_email, r.registered_at,
		s.id, s.name, s.is_active, s.created_at
	FROM registrations r
	INNER JOIN seasons s ON r.season_id = s.id`

	args := []interface{}{}

	// Add season filter if provided
	if seasonID != "" {
		query += " WHERE r.season_id = ?"
		args = append(args, seasonID)
	}

	query += " ORDER BY r.registered_at DESC"

	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query registrations: %w", err)
	}
	defer rows.Close()

	var regs []*Registration
	for rows.Next() {
		reg := &Registration{}
		var season Season
		var seasonIDNull, seasonNameNull sql.NullString
		var seasonIsActiveNull sql.NullBool
		var seasonCreatedAtNull sql.NullTime
		var genderNull sql.NullString

		err := rows.Scan(
			&reg.ID, &reg.SeasonID, &reg.FirstName, &reg.LastName, &reg.Grade, &reg.Teacher, &genderNull,
			&reg.ParentContactNumber, &reg.BackupContactNumber, &reg.ParentEmail, &reg.RegisteredAt,
			&seasonIDNull, &seasonNameNull, &seasonIsActiveNull, &seasonCreatedAtNull,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan registration row: %w", err)
		}

		// Handle NULL gender value
		if genderNull.Valid {
			reg.Gender = genderNull.String
		} else {
			reg.Gender = "" // Use empty string for NULL gender
		}

		// Add season info if available
		if seasonIDNull.Valid && seasonNameNull.Valid {
			season.ID = seasonIDNull.String
			season.Name = seasonNameNull.String
			season.IsActive = seasonIsActiveNull.Bool
			season.CreatedAt = seasonCreatedAtNull.Time
			reg.Season = &season
		}

		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating registration rows: %w", err)
	}

	return regs, nil
}

// GetFilteredRegistrations returns registrations with filtering, pagination, and search functionality
func (db *Database) GetFilteredRegistrations(seasonID, searchQuery string, page, perPage int) ([]*Registration, int, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// Calculate offset
	offset := (page - 1) * perPage

	// Base query
	query := `SELECT
		r.id, r.season_id, r.first_name, r.last_name, r.grade, r.teacher, r.gender,
		r.parent_contact_number, r.backup_contact_number, r.parent_email, 
		r.dismissal_method, r.allergies, r.medical_info, r.registered_at,
		s.id, s.name, s.is_active, s.created_at
	FROM registrations r
	INNER JOIN seasons s ON r.season_id = s.id`

	// Count query for pagination
	countQuery := `SELECT COUNT(*) FROM registrations r`

	// Build where clause
	whereClause := ""
	args := []interface{}{}
	countArgs := []interface{}{}

	// Add season filter if provided
	if seasonID != "" {
		whereClause = " WHERE r.season_id = ?"
		args = append(args, seasonID)
		countArgs = append(countArgs, seasonID)
	}

	// Add search filter if provided
	if searchQuery != "" {
		searchTerm := "%" + searchQuery + "%"
		if whereClause == "" {
			whereClause = " WHERE"
		} else {
			whereClause += " AND"
		}
		whereClause += ` (
			r.first_name LIKE ? OR
			r.last_name LIKE ? OR
			r.grade LIKE ? OR
			r.teacher LIKE ? OR
			r.parent_email LIKE ?
		)`
		args = append(args, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm)
		countArgs = append(countArgs, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm)
	}

	// Complete queries
	query += whereClause + " ORDER BY r.registered_at DESC LIMIT ? OFFSET ?"
	countQuery += whereClause

	// Add pagination parameters
	args = append(args, perPage, offset)

	// Get total count
	var totalCount int
	err := db.db.QueryRow(countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	// Get registrations
	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query registrations: %w", err)
	}
	defer rows.Close()

	var regs []*Registration
	for rows.Next() {
		reg := &Registration{}
		var season Season
		var seasonIDNull, seasonNameNull sql.NullString
		var seasonIsActiveNull sql.NullBool
		var seasonCreatedAtNull sql.NullTime
		var genderNull, dismissalMethodNull, allergiesNull, medicalInfoNull sql.NullString

		err := rows.Scan(
			&reg.ID, &reg.SeasonID, &reg.FirstName, &reg.LastName, &reg.Grade, &reg.Teacher, &genderNull,
			&reg.ParentContactNumber, &reg.BackupContactNumber, &reg.ParentEmail,
			&dismissalMethodNull, &allergiesNull, &medicalInfoNull, &reg.RegisteredAt,
			&seasonIDNull, &seasonNameNull, &seasonIsActiveNull, &seasonCreatedAtNull,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan registration row: %w", err)
		}

		// Handle NULL values
		if genderNull.Valid {
			reg.Gender = genderNull.String
		} else {
			reg.Gender = "" // Use empty string for NULL gender
		}

		if dismissalMethodNull.Valid {
			reg.DismissalMethod = dismissalMethodNull.String
		} else {
			reg.DismissalMethod = ""
		}

		if allergiesNull.Valid {
			reg.Allergies = allergiesNull.String
		} else {
			reg.Allergies = ""
		}

		if medicalInfoNull.Valid {
			reg.MedicalInfo = medicalInfoNull.String
		} else {
			reg.MedicalInfo = ""
		}

		// Add season info if available
		if seasonIDNull.Valid && seasonNameNull.Valid {
			season.ID = seasonIDNull.String
			season.Name = seasonNameNull.String
			season.IsActive = seasonIsActiveNull.Bool
			season.CreatedAt = seasonCreatedAtNull.Time
			reg.Season = &season
		}

		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating registration rows: %w", err)
	}

	return regs, totalCount, nil
}

// SaveTrack saves a track to the database
func (db *Database) SaveTrack(track *Track) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// Begin a transaction
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// If this track is default, unset default on all other tracks for this season
	if track.IsDefault {
		_, err = tx.Exec("UPDATE tracks SET is_default = 0 WHERE season_id = ?", track.SeasonID)
		if err != nil {
			return fmt.Errorf("failed to unset default tracks: %w", err)
		}
	}

	// Insert the new track
	_, err = tx.Exec(
		`INSERT INTO tracks (id, season_id, name, distance_miles, is_default, created_at) 
		VALUES (?, ?, ?, ?, ?, ?)`,
		track.ID, track.SeasonID, track.Name, track.DistanceMiles, track.IsDefault, track.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save track: %w", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetTracksBySeasonID retrieves all tracks for a given season
func (db *Database) GetTracksBySeasonID(seasonID string) ([]*Track, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	rows, err := db.db.Query(
		`SELECT id, season_id, name, distance_miles, is_default, created_at 
		FROM tracks WHERE season_id = ? ORDER BY is_default DESC, name ASC`,
		seasonID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query tracks: %w", err)
	}
	defer rows.Close()

	var tracks []*Track
	for rows.Next() {
		track := &Track{}
		err := rows.Scan(
			&track.ID, &track.SeasonID, &track.Name, &track.DistanceMiles,
			&track.IsDefault, &track.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan track row: %w", err)
		}
		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating track rows: %w", err)
	}

	return tracks, nil
}

// GetDefaultTrackForSeason retrieves the default track for a season
func (db *Database) GetDefaultTrackForSeason(seasonID string) (*Track, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	track := &Track{}
	err := db.db.QueryRow(
		`SELECT id, season_id, name, distance_miles, is_default, created_at 
		FROM tracks WHERE season_id = ? AND is_default = 1`,
		seasonID,
	).Scan(
		&track.ID, &track.SeasonID, &track.Name, &track.DistanceMiles,
		&track.IsDefault, &track.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get default track: %w", err)
	}

	return track, nil
}

// RecordScan records a new scan in the database
func (db *Database) RecordScan(registrationID string, trackID *string) (*ScanRecord, *Registration, error) {
	// Note: Removed mutex lock - SQLite with WAL mode handles concurrency
	// Begin a transaction
	tx, err := db.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Check if the registration exists and get its season ID
	reg := &Registration{}
	var seasonID sql.NullString
	var genderNull sql.NullString
	log.Printf("checking if reg exists")
	err = tx.QueryRow(
		`SELECT
			id, season_id, first_name, last_name, grade, teacher, gender,
			parent_contact_number, backup_contact_number, parent_email, registered_at
		FROM registrations WHERE id = ?`,
		registrationID,
	).Scan(
		&reg.ID, &seasonID, &reg.FirstName, &reg.LastName, &reg.Grade, &reg.Teacher, &genderNull,
		&reg.ParentContactNumber, &reg.BackupContactNumber, &reg.ParentEmail, &reg.RegisteredAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("registration not found")
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get registration: %w", err)
	}

	// Handle NULL gender value
	if genderNull.Valid {
		reg.Gender = genderNull.String
	} else {
		reg.Gender = "" // Use empty string for NULL gender
	}

	// Set the season ID for the registration
	if seasonID.Valid {
		reg.SeasonID = &seasonID.String

		// Get the associated season if it exists
		season := &Season{}
		var seasonRegToken sql.NullString
		log.Printf("checking if season exists")
		err = tx.QueryRow(
			`SELECT id, name, is_active, created_at, registration_token FROM seasons WHERE id = ?`,
			reg.SeasonID,
		).Scan(
			&season.ID, &season.Name, &season.IsActive, &season.CreatedAt, &seasonRegToken,
		)

		if err != nil && err != sql.ErrNoRows {
			return nil, nil, fmt.Errorf("failed to get season: %w", err)
		}

		if err == nil {
			if seasonRegToken.Valid {
				season.RegistrationToken = seasonRegToken.String
			}
			reg.Season = season
		}
	}

	// Get track distance if track ID is provided
	var trackDistance float64
	if trackID != nil && *trackID != "" {
		log.Printf("getting track distance")
		err = tx.QueryRow("SELECT distance_miles FROM tracks WHERE id = ?", *trackID).Scan(&trackDistance)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get track distance: %w", err)
		}
	} else {
		// If no track specified, try to get default track for the season
		if reg.SeasonID != nil {
			log.Printf("getting default track for season")
			err = tx.QueryRow("SELECT distance_miles FROM tracks WHERE season_id = ? AND is_default = 1", *reg.SeasonID).Scan(&trackDistance)
			if err != nil && err != sql.ErrNoRows {
				return nil, nil, fmt.Errorf("failed to get default track distance: %w", err)
			}
		}
	}

	// Check for last scan to implement debounce (5 min/mile minimum pace)
	now := time.Now()
	if trackDistance > 0 {
		var lastScanTime sql.NullTime
		log.Printf("checking last scan time")
		err = tx.QueryRow(
			"SELECT scanned_at FROM scan_records WHERE registration_id = ? ORDER BY scanned_at DESC LIMIT 1",
			registrationID,
		).Scan(&lastScanTime)

		if err == nil && lastScanTime.Valid {
			// Calculate time since last scan
			timeSinceLastScan := now.Sub(lastScanTime.Time)

			// Calculate minimum time required for this distance at 5 min/mile pace
			minimumTime := time.Duration(trackDistance * 5 * float64(time.Minute))

			// Reject if scan is too soon
			if timeSinceLastScan < minimumTime {
				minutesSince := timeSinceLastScan.Minutes()
				pacePerMile := minutesSince / trackDistance
				return nil, nil, fmt.Errorf("scan rejected: too soon since last scan (%.1f minutes ago, %.1f min/mile pace). Minimum pace is 5 min/mile", minutesSince, pacePerMile)
			}
		}
	}

	// Create a new scan record
	scanID := uuid.New().String()

	// Insert the scan record with season ID and track ID
	log.Printf("inserting scan record")
	_, err = tx.Exec(
		`INSERT INTO scan_records (id, registration_id, season_id, track_id, scanned_at) 
		VALUES (?, ?, ?, ?, ?)`,
		scanID, registrationID, reg.SeasonID, trackID, now,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to insert scan record: %w", err)
	}

	log.Printf("committing transaction")
	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Create the scan record to return
	scan := &ScanRecord{
		ID:             scanID,
		RegistrationID: registrationID,
		SeasonID:       *reg.SeasonID,
		TrackID:        trackID,
		ScannedAt:      now,
		Season:         reg.Season,
	}

	// If track ID was provided, fetch the track info
	log.Printf("getting track info")
	if trackID != nil && *trackID != "" {
		track := &Track{}
		err = db.db.QueryRow(
			`SELECT id, season_id, name, distance_miles, is_default, created_at 
			FROM tracks WHERE id = ?`,
			*trackID,
		).Scan(
			&track.ID, &track.SeasonID, &track.Name, &track.DistanceMiles,
			&track.IsDefault, &track.CreatedAt,
		)
		if err == nil {
			scan.Track = track
		}
	}

	return scan, reg, nil
}

// GetScansByRegistrationID retrieves all scans for a given registration ID
func (db *Database) GetScansByRegistrationID(registrationID string) ([]*ScanRecord, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	rows, err := db.db.Query(
		`SELECT sr.id, sr.registration_id, sr.season_id, sr.scanned_at,
		s.id, s.name, s.is_active, s.created_at
		FROM scan_records sr
		LEFT JOIN seasons s ON sr.season_id = s.id
		WHERE sr.registration_id = ?
		ORDER BY sr.scanned_at DESC`,
		registrationID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query scans: %w", err)
	}
	defer rows.Close()

	var scans []*ScanRecord
	for rows.Next() {
		scan := &ScanRecord{}
		var seasonIDNull, seasonNameNull sql.NullString
		var seasonIsActiveNull sql.NullBool
		var seasonCreatedAtNull sql.NullTime

		err := rows.Scan(
			&scan.ID, &scan.RegistrationID, &scan.SeasonID, &scan.ScannedAt,
			&seasonIDNull, &seasonNameNull, &seasonIsActiveNull, &seasonCreatedAtNull,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Set season if available
		if seasonIDNull.Valid && seasonNameNull.Valid {
			scan.Season = &Season{
				ID:        seasonIDNull.String,
				Name:      seasonNameNull.String,
				IsActive:  seasonIsActiveNull.Bool,
				CreatedAt: seasonCreatedAtNull.Time,
			}
		}

		scans = append(scans, scan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scan rows: %w", err)
	}

	return scans, nil
}

// GetAllScans returns all scan records, optionally filtered by season
func (db *Database) GetAllScans(seasonID string) ([]*ScanRecord, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	query := `SELECT sr.id, sr.registration_id, sr.season_id, sr.scanned_at,
		r.first_name, r.last_name,
		s.id, s.name, s.is_active, s.created_at
	FROM scan_records sr
	JOIN registrations r ON sr.registration_id = r.id
	LEFT JOIN seasons s ON sr.season_id = s.id`

	args := []interface{}{}

	// Add season filter if provided
	if seasonID != "" {
		query += " WHERE sr.season_id = ?"
		args = append(args, seasonID)
	}

	query += " ORDER BY sr.scanned_at DESC"

	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scans: %w", err)
	}
	defer rows.Close()

	var scans []*ScanRecord
	for rows.Next() {
		scan := &ScanRecord{}
		var firstName, lastName string
		var seasonIDNull, seasonNameNull sql.NullString
		var seasonIsActiveNull sql.NullBool
		var seasonCreatedAtNull sql.NullTime

		err := rows.Scan(
			&scan.ID, &scan.RegistrationID, &scan.SeasonID, &scan.ScannedAt,
			&firstName, &lastName,
			&seasonIDNull, &seasonNameNull, &seasonIsActiveNull, &seasonCreatedAtNull,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Add runner name for convenience
		scan.RunnerName = fmt.Sprintf("%s %s", firstName, lastName)

		// Set season if available
		if seasonIDNull.Valid && seasonNameNull.Valid {
			scan.Season = &Season{
				ID:        seasonIDNull.String,
				Name:      seasonNameNull.String,
				IsActive:  seasonIsActiveNull.Bool,
				CreatedAt: seasonCreatedAtNull.Time,
			}
		}

		scans = append(scans, scan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scan rows: %w", err)
	}

	return scans, nil
}

// GetRegistrationCountForSeason returns the count of registrations for a given season
func (db *Database) GetRegistrationCountForSeason(seasonID string) (int, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM registrations WHERE season_id = ?", seasonID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count registrations for season: %w", err)
	}

	return count, nil
}

// GetScanCountForSeason returns the count of scans for a given season
func (db *Database) GetScanCountForSeason(seasonID string) (int, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM scan_records WHERE season_id = ?", seasonID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scans for season: %w", err)
	}

	return count, nil
}

// GetSeasonStatistics returns comprehensive statistics for a season
func (db *Database) GetSeasonStatistics(seasonID string) (*SeasonStats, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	stats := &SeasonStats{
		TrackUsage: make(map[string]int),
	}

	// Get total runners
	err := db.db.QueryRow(
		"SELECT COUNT(DISTINCT id) FROM registrations WHERE season_id = ?",
		seasonID,
	).Scan(&stats.TotalRunners)
	if err != nil {
		return nil, fmt.Errorf("failed to get total runners: %w", err)
	}

	// Get total runs and distance
	err = db.db.QueryRow(`
		SELECT 
			COUNT(*) as total_runs,
			COALESCE(SUM(t.distance_miles), 0) as total_distance
		FROM scan_records sr
		LEFT JOIN tracks t ON sr.track_id = t.id
		WHERE sr.season_id = ?
	`, seasonID).Scan(&stats.TotalRuns, &stats.TotalDistance)
	if err != nil {
		return nil, fmt.Errorf("failed to get total runs and distance: %w", err)
	}

	// Calculate average per run
	if stats.TotalRuns > 0 {
		stats.AveragePerRun = stats.TotalDistance / float64(stats.TotalRuns)
	}

	// Get grade statistics
	gradeRows, err := db.db.Query(`
		SELECT 
			r.grade,
			COUNT(DISTINCT r.id) as runner_count,
			COUNT(sr.id) as total_runs,
			COALESCE(SUM(t.distance_miles), 0) as total_distance
		FROM registrations r
		LEFT JOIN scan_records sr ON r.id = sr.registration_id
		LEFT JOIN tracks t ON sr.track_id = t.id
		WHERE r.season_id = ?
		GROUP BY r.grade
		ORDER BY r.grade
	`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("failed to get grade statistics: %w", err)
	}
	defer gradeRows.Close()

	var gradeStats []GradeStats
	for gradeRows.Next() {
		var gs GradeStats
		err := gradeRows.Scan(&gs.Grade, &gs.RunnerCount, &gs.TotalRuns, &gs.TotalDistance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan grade stats: %w", err)
		}
		gradeStats = append(gradeStats, gs)
	}

	// Get top 3 runners for each grade
	for i := range gradeStats {
		topRunners, err := db.getTopRunnersForGrade(seasonID, gradeStats[i].Grade, 3)
		if err != nil {
			return nil, fmt.Errorf("failed to get top runners for grade %s: %w", gradeStats[i].Grade, err)
		}
		gradeStats[i].TopRunners = topRunners
	}
	stats.GradeStats = gradeStats

	// Get overall top 10 runners
	topRunners, err := db.getTopRunners(seasonID, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get top runners: %w", err)
	}
	stats.TopRunners = topRunners

	// Get track usage
	trackRows, err := db.db.Query(`
		SELECT 
			t.name,
			COUNT(*) as usage_count
		FROM scan_records sr
		JOIN tracks t ON sr.track_id = t.id
		WHERE sr.season_id = ?
		GROUP BY t.id, t.name
		ORDER BY usage_count DESC
	`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("failed to get track usage: %w", err)
	}
	defer trackRows.Close()

	for trackRows.Next() {
		var trackName string
		var count int
		err := trackRows.Scan(&trackName, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan track usage: %w", err)
		}
		stats.TrackUsage[trackName] = count
	}

	return stats, nil
}

// getTopRunnersForGrade returns the top N runners for a specific grade
func (db *Database) getTopRunnersForGrade(seasonID, grade string, limit int) ([]RunnerStats, error) {
	rows, err := db.db.Query(`
		SELECT 
			r.id,
			r.first_name,
			r.last_name,
			r.grade,
			r.teacher,
			COUNT(sr.id) as run_count,
			COALESCE(SUM(t.distance_miles), 0) as total_distance
		FROM registrations r
		LEFT JOIN scan_records sr ON r.id = sr.registration_id
		LEFT JOIN tracks t ON sr.track_id = t.id
		WHERE r.season_id = ? AND r.grade = ?
		GROUP BY r.id, r.first_name, r.last_name, r.grade, r.teacher
		HAVING run_count > 0
		ORDER BY total_distance DESC, run_count DESC
		LIMIT ?
	`, seasonID, grade, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runners []RunnerStats
	for rows.Next() {
		var rs RunnerStats
		err := rows.Scan(
			&rs.RegistrationID, &rs.FirstName, &rs.LastName,
			&rs.Grade, &rs.Teacher, &rs.RunCount, &rs.TotalDistance,
		)
		if err != nil {
			return nil, err
		}
		runners = append(runners, rs)
	}

	return runners, nil
}

// getTopRunners returns the top N runners overall
func (db *Database) getTopRunners(seasonID string, limit int) ([]RunnerStats, error) {
	rows, err := db.db.Query(`
		SELECT 
			r.id,
			r.first_name,
			r.last_name,
			r.grade,
			r.teacher,
			COUNT(sr.id) as run_count,
			COALESCE(SUM(t.distance_miles), 0) as total_distance
		FROM registrations r
		LEFT JOIN scan_records sr ON r.id = sr.registration_id
		LEFT JOIN tracks t ON sr.track_id = t.id
		WHERE r.season_id = ?
		GROUP BY r.id, r.first_name, r.last_name, r.grade, r.teacher
		HAVING run_count > 0
		ORDER BY total_distance DESC, run_count DESC
		LIMIT ?
	`, seasonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runners []RunnerStats
	for rows.Next() {
		var rs RunnerStats
		err := rows.Scan(
			&rs.RegistrationID, &rs.FirstName, &rs.LastName,
			&rs.Grade, &rs.Teacher, &rs.RunCount, &rs.TotalDistance,
		)
		if err != nil {
			return nil, err
		}
		runners = append(runners, rs)
	}

	return runners, nil
}
