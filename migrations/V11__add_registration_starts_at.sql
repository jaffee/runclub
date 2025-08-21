-- Migration: Add registration start date to seasons

-- Add column to seasons table for when registration becomes active
ALTER TABLE seasons ADD COLUMN registration_starts_at TIMESTAMP;

-- Set existing seasons to have registration already started (in the past)
UPDATE seasons SET registration_starts_at = created_at WHERE registration_starts_at IS NULL;