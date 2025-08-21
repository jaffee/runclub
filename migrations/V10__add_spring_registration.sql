-- Migration: Add spring season registration option

-- Add column to seasons table to enable/disable spring registration option
ALTER TABLE seasons ADD COLUMN spring_registration_enabled BOOLEAN NOT NULL DEFAULT 0;

-- Add column to registrations table to track if user opted for spring registration
ALTER TABLE registrations ADD COLUMN register_for_spring BOOLEAN NOT NULL DEFAULT 0;

-- Create index for finding runners who opted for spring registration
CREATE INDEX IF NOT EXISTS idx_registrations_spring ON registrations(season_id, register_for_spring);