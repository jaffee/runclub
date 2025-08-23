-- Migration: Add max_registrations field to seasons table

-- Add max_registrations column with default value of 95
-- This will automatically set all existing seasons to 95
ALTER TABLE seasons ADD COLUMN max_registrations INTEGER NOT NULL DEFAULT 95;