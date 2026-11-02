-- Text that people type or scrape had a length limit here that SQLite doesn't
-- enforce, so it saved on SQLite and failed on PostgreSQL. These now match the
-- other titles and names (text), and the other URL tables (2048).
ALTER TABLE stories ALTER COLUMN title TYPE text;
ALTER TABLE audios ALTER COLUMN title TYPE text;
ALTER TABLE tags ALTER COLUMN sort_name TYPE text;
ALTER TABLE story_urls ALTER COLUMN url TYPE varchar(2048);
ALTER TABLE audio_urls ALTER COLUMN url TYPE varchar(2048);
