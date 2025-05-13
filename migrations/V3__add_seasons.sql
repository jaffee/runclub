-- Migration: Add seasons support

-- Create seasons table
CREATE TABLE IF NOT EXISTS seasons (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Add season_id column to registrations table
ALTER TABLE registrations ADD COLUMN season_id TEXT REFERENCES seasons(id);

-- Add season_id column to scan_records table 
ALTER TABLE scan_records ADD COLUMN season_id TEXT REFERENCES seasons(id);

-- Index for filtering by season
CREATE INDEX IF NOT EXISTS idx_registrations_season_id ON registrations(season_id);
CREATE INDEX IF NOT EXISTS idx_scan_records_season_id ON scan_records(season_id);

-- Ensure only one active season at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_seasons_active ON seasons(is_active) WHERE is_active = 1;