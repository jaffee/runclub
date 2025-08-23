-- Add privacy opt-out fields to registrations table
ALTER TABLE registrations ADD COLUMN opt_out_website_display BOOLEAN DEFAULT FALSE;
ALTER TABLE registrations ADD COLUMN opt_out_photo_sharing BOOLEAN DEFAULT FALSE;