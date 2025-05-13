-- Migration: Initial database schema

-- Registrations table
CREATE TABLE IF NOT EXISTS registrations (
    id TEXT PRIMARY KEY,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    grade TEXT NOT NULL,
    teacher TEXT NOT NULL,
    parent_contact_number TEXT NOT NULL,
    backup_contact_number TEXT,
    parent_email TEXT NOT NULL,
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Scan records table
CREATE TABLE IF NOT EXISTS scan_records (
    id TEXT PRIMARY KEY,
    registration_id TEXT NOT NULL,
    scanned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (registration_id) REFERENCES registrations(id)
);

-- Index for faster lookup of scans by registration ID
CREATE INDEX IF NOT EXISTS idx_scan_records_registration_id ON scan_records(registration_id);

-- Index for sorting scans by time
CREATE INDEX IF NOT EXISTS idx_scan_records_scanned_at ON scan_records(scanned_at);