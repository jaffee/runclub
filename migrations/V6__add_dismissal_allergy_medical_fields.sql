-- Add dismissal method, allergies, and medical info fields to registrations table
ALTER TABLE registrations ADD COLUMN dismissal_method TEXT;
ALTER TABLE registrations ADD COLUMN allergies TEXT;
ALTER TABLE registrations ADD COLUMN medical_info TEXT;