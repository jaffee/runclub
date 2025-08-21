-- Add parent first and last name fields to registrations table
ALTER TABLE registrations ADD COLUMN parent_first_name TEXT;
ALTER TABLE registrations ADD COLUMN parent_last_name TEXT;