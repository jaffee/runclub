package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDatabaseOperations(t *testing.T) {
	// Remove any existing test database
	os.Remove("test_runclub.db")
	os.Remove("runclub.db")

	// Initialize database
	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()
	defer os.Remove("runclub.db") // Clean up after test

	// Create a test season first
	testSeason := &Season{
		ID:        uuid.New().String(),
		Name:      "Test Season",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	// Save the season
	err = db.SaveSeason(testSeason)
	if err != nil {
		t.Fatalf("Failed to create test season: %v", err)
	}

	// Test registration operations
	t.Run("Registration Operations", func(t *testing.T) {
		// Create test registration with the season
		seasonID := testSeason.ID
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            &seasonID,
			FirstName:           "John",
			LastName:            "Doe",
			Grade:               "5",
			Teacher:             "Mrs. Smith",
			Gender:              "Male",
			ParentContactNumber: "555-1234",
			BackupContactNumber: "555-5678",
			ParentEmail:         "parent@example.com",
			RegisteredAt:        time.Now(),
			Season:              testSeason,
		}

		// Save registration
		err := db.SaveRegistration(reg)
		if err != nil {
			t.Fatalf("Failed to save registration: %v", err)
		}

		// Retrieve registration
		retrieved, exists, err := db.GetRegistration(reg.ID)
		if err != nil {
			t.Fatalf("Error retrieving registration: %v", err)
		}
		if !exists {
			t.Fatalf("Registration should exist but doesn't")
		}
		if retrieved.FirstName != reg.FirstName || retrieved.LastName != reg.LastName {
			t.Errorf("Retrieved registration data doesn't match: got %s %s, want %s %s",
				retrieved.FirstName, retrieved.LastName, reg.FirstName, reg.LastName)
		}

		// Get all registrations
		allRegs, err := db.GetAllRegistrations("")
		if err != nil {
			t.Fatalf("Failed to get all registrations: %v", err)
		}
		if len(allRegs) != 1 {
			t.Errorf("Expected 1 registration, got %d", len(allRegs))
		}
	})

	// Test scan operations
	t.Run("Scan Operations", func(t *testing.T) {
		// Create test registration with the season
		seasonID := testSeason.ID
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            &seasonID,
			FirstName:           "Jane",
			LastName:            "Smith",
			Grade:               "3",
			Teacher:             "Mr. Johnson",
			Gender:              "Female",
			ParentContactNumber: "555-9876",
			BackupContactNumber: "555-6543",
			ParentEmail:         "parent2@example.com",
			RegisteredAt:        time.Now(),
			Season:              testSeason,
		}

		// Save registration
		err := db.SaveRegistration(reg)
		if err != nil {
			t.Fatalf("Failed to save registration: %v", err)
		}

		// Record a scan
		scan, regRetrieved, err := db.RecordScan(reg.ID, nil)
		if err != nil {
			t.Fatalf("Failed to record scan: %v", err)
		}
		if scan.RegistrationID != reg.ID {
			t.Errorf("Scan registration ID mismatch: got %s, want %s", scan.RegistrationID, reg.ID)
		}
		if regRetrieved.ID != reg.ID {
			t.Errorf("Retrieved registration ID mismatch: got %s, want %s", regRetrieved.ID, reg.ID)
		}

		// Get scans by registration ID
		scans, err := db.GetScansByRegistrationID(reg.ID)
		if err != nil {
			t.Fatalf("Failed to get scans by registration ID: %v", err)
		}
		if len(scans) != 1 {
			t.Errorf("Expected 1 scan, got %d", len(scans))
		}

		// Get all scans
		allScans, err := db.GetAllScans("")
		if err != nil {
			t.Fatalf("Failed to get all scans: %v", err)
		}
		if len(allScans) != 1 {
			t.Errorf("Expected 1 scan, got %d", len(allScans))
		}
		if allScans[0].RunnerName != fmt.Sprintf("%s %s", reg.FirstName, reg.LastName) {
			t.Errorf("Runner name mismatch: got %s, want %s %s",
				allScans[0].RunnerName, reg.FirstName, reg.LastName)
		}

		// Record another scan for the same registration
		time.Sleep(time.Millisecond * 100) // Ensure different timestamps
		_, _, err = db.RecordScan(reg.ID, nil)
		if err != nil {
			t.Fatalf("Failed to record second scan: %v", err)
		}

		// Get scans by registration ID again
		scans, err = db.GetScansByRegistrationID(reg.ID)
		if err != nil {
			t.Fatalf("Failed to get scans by registration ID: %v", err)
		}
		if len(scans) != 2 {
			t.Errorf("Expected 2 scans, got %d", len(scans))
		}
	})

	// Test nonexistent registration
	t.Run("Nonexistent Registration", func(t *testing.T) {
		_, exists, err := db.GetRegistration("nonexistent-id")
		if err != nil {
			t.Fatalf("Error retrieving nonexistent registration: %v", err)
		}
		if exists {
			t.Errorf("Nonexistent registration should not exist")
		}

		_, _, err = db.RecordScan("nonexistent-id", nil)
		if err == nil {
			t.Errorf("Expected error when recording scan for nonexistent registration")
		}
	})
}