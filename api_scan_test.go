package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
)

func setupTestDatabase(t *testing.T) (*Database, func()) {
	// Create a temporary database file
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Initialize database
	os.Setenv("DATABASE_PATH", tmpfile.Name())
	db, err := NewDatabase()
	if err != nil {
		os.Remove(tmpfile.Name())
		t.Fatal(err)
	}

	// Create test season
	seasonID := uuid.New().String()
	season := &Season{
		ID:        seasonID,
		Name:      "Test Season",
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	err = db.SaveSeason(season)
	if err != nil {
		db.Close()
		os.Remove(tmpfile.Name())
		t.Fatal(err)
	}

	// Create test track
	track := &Track{
		ID:            uuid.New().String(),
		SeasonID:      seasonID,
		Name:          "Test Track",
		DistanceMiles: 1.0,
		IsDefault:     true,
		CreatedAt:     time.Now(),
	}
	err = db.SaveTrack(track)
	if err != nil {
		db.Close()
		os.Remove(tmpfile.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpfile.Name())
		os.Unsetenv("DATABASE_PATH")
	}

	return db, cleanup
}

func createTestRegistration(t *testing.T, db *Database, seasonID string) *Registration {
	reg := &Registration{
		ID:                  uuid.New().String(),
		SeasonID:            &seasonID,
		FirstName:           "Test",
		LastName:            "Runner",
		Grade:               "3",
		Teacher:             "Ms. Smith",
		Gender:              "Male",
		ParentContactNumber: "555-1234",
		BackupContactNumber: "555-5678",
		ParentEmail:         "parent@example.com",
		DismissalMethod:     "Picked up by adult",
		RegisteredAt:        time.Now(),
	}

	err := db.SaveRegistration(reg)
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

func TestAPIScanHandlerWithRealDatabase(t *testing.T) {
	// Save original database and restore after tests
	originalDB := database
	defer func() { database = originalDB }()

	// Setup test database
	db, cleanup := setupTestDatabase(t)
	defer cleanup()
	database = db

	// Get the active season
	activeSeason, _, err := db.GetActiveSeason()
	if err != nil {
		t.Fatal(err)
	}

	// Create a test registration
	reg := createTestRegistration(t, db, activeSeason.ID)

	// Initialize test session store
	store = sessions.NewCookieStore([]byte("test-secret"))

	tests := []struct {
		name           string
		body           map[string]interface{}
		expectedStatus int
		expectedResult ScanResult
	}{
		{
			name: "Valid scan",
			body: map[string]interface{}{
				"code": reg.ID,
			},
			expectedStatus: http.StatusOK,
			expectedResult: ScanResult{
				Success: true,
				Message: fmt.Sprintf("Successfully recorded run for %s %s", reg.FirstName, reg.LastName),
			},
		},
		{
			name: "Invalid UUID",
			body: map[string]interface{}{
				"code": "not-a-uuid",
			},
			expectedStatus: http.StatusOK,
			expectedResult: ScanResult{
				Success: false,
				Message: "Invalid QR code format",
			},
		},
		{
			name: "Non-existent runner",
			body: map[string]interface{}{
				"code": uuid.New().String(),
			},
			expectedStatus: http.StatusOK,
			expectedResult: ScanResult{
				Success: false,
				Message: "Runner not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/scan", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			// Create authenticated session
			session, _ := store.Get(req, "run-club-session")
			session.Values["authenticated"] = true
			session.Values["username"] = "testuser"
			session.Values["role"] = RoleScanner

			rr := httptest.NewRecorder()

			// Create handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				session.Save(r, w)
				apiScanHandler(w, r)
			})

			handler.ServeHTTP(rr, req)

			// Check status
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Check response
			var result ScanResult
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if result.Success != tt.expectedResult.Success {
				t.Errorf("Expected success=%v, got %v", tt.expectedResult.Success, result.Success)
			}

			if result.Message != tt.expectedResult.Message {
				t.Errorf("Expected message=%q, got %q", tt.expectedResult.Message, result.Message)
			}
		})
	}
}

func TestAPIScanHandlerConcurrentRequests(t *testing.T) {
	// Save original database and restore after tests
	originalDB := database
	defer func() { database = originalDB }()

	// Setup test database
	db, cleanup := setupTestDatabase(t)
	defer cleanup()
	database = db

	// Get the active season
	activeSeason, _, err := db.GetActiveSeason()
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple test registrations
	var registrations []*Registration
	for i := 0; i < 10; i++ {
		reg := createTestRegistration(t, db, activeSeason.ID)
		registrations = append(registrations, reg)
	}

	// Initialize test session store
	store = sessions.NewCookieStore([]byte("test-secret"))

	// Test concurrent scans
	var wg sync.WaitGroup
	errors := make(chan error, len(registrations)*2)

	// Run 2 scans per registration concurrently
	for _, reg := range registrations {
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(regID string, attempt int) {
				defer wg.Done()

				body := map[string]interface{}{
					"code": regID,
				}
				bodyBytes, _ := json.Marshal(body)

				req := httptest.NewRequest(http.MethodPost, "/api/scan", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")

				// Create authenticated session
				session, _ := store.Get(req, "run-club-session")
				session.Values["authenticated"] = true
				session.Values["username"] = "testuser"
				session.Values["role"] = RoleScanner

				rr := httptest.NewRecorder()

				// Add timeout to detect hangs
				done := make(chan bool)
				go func() {
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						session.Save(r, w)
						apiScanHandler(w, r)
					})
					handler.ServeHTTP(rr, req)
					done <- true
				}()

				select {
				case <-done:
					// Success
					if rr.Code != http.StatusOK {
						errors <- fmt.Errorf("unexpected status code: %d", rr.Code)
					}
				case <-time.After(5 * time.Second):
					errors <- fmt.Errorf("request timed out for registration %s", regID)
				}
			}(reg.ID, i)

			// Small delay between requests to spread them out
			time.Sleep(10 * time.Millisecond)
		}
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errorCount int
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent request error: %v", err)
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Total errors: %d out of %d requests", errorCount, len(registrations)*2)
	}
}

func TestAPIScanHandlerDatabaseLockContention(t *testing.T) {
	// Save original database and restore after tests
	originalDB := database
	defer func() { database = originalDB }()

	// Setup test database
	db, cleanup := setupTestDatabase(t)
	defer cleanup()
	database = db

	// Get the active season
	activeSeason, _, err := db.GetActiveSeason()
	if err != nil {
		t.Fatal(err)
	}

	// Create a test registration
	reg := createTestRegistration(t, db, activeSeason.ID)

	// Initialize test session store
	store = sessions.NewCookieStore([]byte("test-secret"))

	// Start multiple concurrent scans for the same registration
	// This should trigger debounce logic and potentially expose lock issues
	var wg sync.WaitGroup
	concurrency := 10
	results := make(chan string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(attempt int) {
			defer wg.Done()

			body := map[string]interface{}{
				"code": reg.ID,
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/scan", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			// Create authenticated session
			session, _ := store.Get(req, "run-club-session")
			session.Values["authenticated"] = true
			session.Values["username"] = fmt.Sprintf("testuser%d", attempt)
			session.Values["role"] = RoleScanner

			rr := httptest.NewRecorder()

			// Measure request duration
			start := time.Now()
			
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				session.Save(r, w)
				apiScanHandler(w, r)
			})
			
			handler.ServeHTTP(rr, req)
			duration := time.Since(start)

			// Parse response
			var result ScanResult
			err := json.Unmarshal(rr.Body.Bytes(), &result)
			if err != nil {
				results <- fmt.Sprintf("attempt %d: failed to parse response after %v", attempt, duration)
				return
			}

			if result.Success {
				results <- fmt.Sprintf("attempt %d: success after %v", attempt, duration)
			} else {
				results <- fmt.Sprintf("attempt %d: %s after %v", attempt, result.Message, duration)
			}
		}(i)
	}

	wg.Wait()
	close(results)

	// Analyze results
	successCount := 0
	debounceCount := 0

	t.Log("Results from concurrent scans:")
	for result := range results {
		t.Log(result)
		if strings.Contains(result, "success") {
			successCount++
		} else if strings.Contains(result, "too soon") {
			debounceCount++
		}
	}

	t.Logf("Successes: %d, Debounced: %d, Total: %d", successCount, debounceCount, concurrency)

	// We expect exactly 1 success and the rest to be debounced
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful scan, got %d", successCount)
	}
}

func TestAPIScanHandlerSlowQueries(t *testing.T) {
	// Save original database and restore after tests
	originalDB := database
	defer func() { database = originalDB }()

	// Setup test database
	db, cleanup := setupTestDatabase(t)
	defer cleanup()
	database = db

	// Get the active season
	activeSeason, _, err := db.GetActiveSeason()
	if err != nil {
		t.Fatal(err)
	}

	// Create many registrations to slow down queries
	t.Log("Creating test data...")
	for i := 0; i < 100; i++ {
		reg := createTestRegistration(t, db, activeSeason.ID)
		
		// Add some scan records for each registration
		for j := 0; j < 5; j++ {
			// Use raw SQL to bypass the debounce logic
			_, err := db.db.Exec(
				`INSERT INTO scan_records (id, registration_id, season_id, scanned_at) 
				VALUES (?, ?, ?, ?)`,
				uuid.New().String(), reg.ID, activeSeason.ID, time.Now().Add(-time.Hour*time.Duration(j+1)),
			)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Create one more registration to test with
	testReg := createTestRegistration(t, db, activeSeason.ID)

	// Initialize test session store
	store = sessions.NewCookieStore([]byte("test-secret"))

	// Test scan with lots of data in database
	body := map[string]interface{}{
		"code": testReg.ID,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/scan", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// Create authenticated session
	session, _ := store.Get(req, "run-club-session")
	session.Values["authenticated"] = true
	session.Values["username"] = "testuser"
	session.Values["role"] = RoleScanner

	rr := httptest.NewRecorder()

	// Measure request duration
	start := time.Now()
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Save(r, w)
		apiScanHandler(w, r)
	})
	
	// Run with timeout
	done := make(chan bool)
	go func() {
		handler.ServeHTTP(rr, req)
		done <- true
	}()

	select {
	case <-done:
		duration := time.Since(start)
		t.Logf("Request completed in %v", duration)
		
		// Check if it took too long (potential hang)
		if duration > 2*time.Second {
			t.Errorf("Request took too long: %v", duration)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Request timed out - possible hang detected")
	}

	// Check response
	var result ScanResult
	err = json.Unmarshal(rr.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got error: %s", result.Message)
	}
}