-- Existing covers have unknown origin and remain protected.
ALTER TABLE scenes ADD COLUMN cover_generated BOOLEAN NOT NULL DEFAULT 0;
