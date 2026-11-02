-- Values longer than the old limits are cut to fit.
ALTER TABLE audio_urls ALTER COLUMN url TYPE varchar(255) USING left(url, 255);
ALTER TABLE story_urls ALTER COLUMN url TYPE varchar(255) USING left(url, 255);
ALTER TABLE tags ALTER COLUMN sort_name TYPE varchar(255) USING left(sort_name, 255);
ALTER TABLE audios ALTER COLUMN title TYPE varchar(255) USING left(title, 255);
ALTER TABLE stories ALTER COLUMN title TYPE varchar(255) USING left(title, 255);
