-- Create tracks table
CREATE TABLE tracks (
    id TEXT PRIMARY KEY,
    season_id TEXT NOT NULL,
    name TEXT NOT NULL,
    distance_miles REAL NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (season_id) REFERENCES seasons(id)
);

-- Create index on season_id for faster lookups
CREATE INDEX idx_tracks_season_id ON tracks(season_id);

-- Add track_id to scan_records table
ALTER TABLE scan_records ADD COLUMN track_id TEXT REFERENCES tracks(id);

-- Create index on track_id for faster lookups
CREATE INDEX idx_scan_records_track_id ON scan_records(track_id);