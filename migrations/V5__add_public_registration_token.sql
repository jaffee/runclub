-- Add public registration token to seasons table
ALTER TABLE seasons ADD COLUMN registration_token TEXT;

-- Create unique index on registration token
CREATE UNIQUE INDEX idx_seasons_registration_token ON seasons(registration_token) WHERE registration_token IS NOT NULL;

-- Generate tokens for existing seasons
UPDATE seasons 
SET registration_token = lower(hex(randomblob(16)))
WHERE registration_token IS NULL;