-- Migration: Add index for names

-- Add an index for searching by names
CREATE INDEX IF NOT EXISTS idx_registrations_names 
ON registrations(first_name, last_name);