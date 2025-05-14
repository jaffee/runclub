-- Migration: Add gender column to registrations table

-- Add gender column to registrations table
ALTER TABLE registrations ADD COLUMN gender TEXT;

-- Update existing records to have a default value (can be null)
-- UPDATE registrations SET gender = NULL WHERE gender IS NULL;