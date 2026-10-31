-- Bring the PostgreSQL backend through the metadata introduced after SQLite 76.
-- Endpoint identifiers are strings in the public API and may be non-UUIDs.
ALTER TABLE performer_stash_ids ALTER COLUMN stash_id TYPE text USING stash_id::text;
ALTER TABLE scene_stash_ids ALTER COLUMN stash_id TYPE text USING stash_id::text;
ALTER TABLE studio_stash_ids ALTER COLUMN stash_id TYPE text USING stash_id::text;
ALTER TABLE tag_stash_ids ALTER COLUMN stash_id TYPE text USING stash_id::text;
CREATE TABLE studio_custom_fields (studio_id integer NOT NULL REFERENCES studios(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(studio_id, field));
CREATE TABLE tag_custom_fields (tag_id integer NOT NULL REFERENCES tags(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(tag_id, field));
CREATE TABLE scene_custom_fields (scene_id integer NOT NULL REFERENCES scenes(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(scene_id, field));
CREATE TABLE gallery_custom_fields (gallery_id integer NOT NULL REFERENCES galleries(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(gallery_id, field));
CREATE TABLE group_custom_fields (group_id integer NOT NULL REFERENCES groups(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(group_id, field));
CREATE TABLE image_custom_fields (image_id integer NOT NULL REFERENCES images(id) ON DELETE CASCADE, field text NOT NULL, value jsonb NOT NULL, type text, PRIMARY KEY(image_id, field));
CREATE INDEX studio_custom_fields_field_value ON studio_custom_fields(field,value);
CREATE INDEX tag_custom_fields_field_value ON tag_custom_fields(field,value);
CREATE INDEX scene_custom_fields_field_value ON scene_custom_fields(field,value);
CREATE INDEX gallery_custom_fields_field_value ON gallery_custom_fields(field,value);
CREATE INDEX group_custom_fields_field_value ON group_custom_fields(field,value);
CREATE INDEX image_custom_fields_field_value ON image_custom_fields(field,value);

ALTER TABLE performers ADD COLUMN career_start date, ADD COLUMN career_start_precision smallint, ADD COLUMN career_end date, ADD COLUMN career_end_precision smallint;
UPDATE performers SET career_start = make_date(substring(career_length FROM '^\s*(\d{4})')::integer,1,1), career_start_precision = 2 WHERE career_length ~ '^\s*\d{4}' AND substring(career_length FROM '^\s*(\d{4})')::integer BETWEEN 1 AND 9999;
UPDATE performers SET career_end = make_date(substring(career_length FROM '-\s*(\d{4})')::integer,1,1), career_end_precision = 2 WHERE career_length ~ '-\s*\d{4}' AND substring(career_length FROM '-\s*(\d{4})')::integer BETWEEN 1 AND 9999;
ALTER TABLE performers DROP COLUMN career_length;
ALTER TABLE studios ADD COLUMN organized boolean NOT NULL DEFAULT false;
ALTER TABLE folders ADD COLUMN basename text;
UPDATE folders SET basename = regexp_replace(rtrim(path, '/\'), '^.*[/\]', '');
ALTER TABLE folders ALTER COLUMN basename SET NOT NULL;
CREATE UNIQUE INDEX folders_parent_basename_unique ON folders(parent_folder_id,basename);
CREATE INDEX folders_basename_index ON folders(basename);
ALTER TABLE scenes ADD COLUMN production_date date, ADD COLUMN production_date_precision smallint, ADD COLUMN cover_generated boolean NOT NULL DEFAULT false;
DELETE FROM files_fingerprints WHERE type = 'phash' AND file_id IN (SELECT file_id FROM video_files WHERE duration > 0 AND duration <= 150);
ALTER TABLE tags ADD COLUMN rating smallint;
ALTER TABLE video_files ADD COLUMN projection text, ADD COLUMN stereo_mode text, ADD COLUMN vr_corrections text, ADD COLUMN vr_passthrough text;
CREATE TABLE group_aliases (group_id integer NOT NULL REFERENCES groups(id) ON DELETE CASCADE, alias text NOT NULL, PRIMARY KEY(group_id,alias));
INSERT INTO group_aliases(group_id,alias) SELECT id, trim(alias) FROM groups CROSS JOIN LATERAL unnest(string_to_array(aliases,',')) AS alias WHERE trim(alias) <> '' ON CONFLICT DO NOTHING;
CREATE INDEX group_aliases_alias_index ON group_aliases(alias);
ALTER TABLE groups DROP COLUMN aliases;
ALTER TABLE files ADD COLUMN missing_since timestamptz;
ALTER TABLE folders ADD COLUMN missing_since timestamptz;
CREATE INDEX files_missing_since_index ON files(missing_since) WHERE missing_since IS NOT NULL;
CREATE INDEX folders_missing_since_index ON folders(missing_since) WHERE missing_since IS NOT NULL;
ALTER TABLE galleries ADD COLUMN o_counter integer NOT NULL DEFAULT 0;

CREATE TABLE voice_actors (
 id serial PRIMARY KEY, name text NOT NULL CHECK(length(trim(name))>0), details text NOT NULL DEFAULT '',
 aliases jsonb NOT NULL DEFAULT '[]', urls jsonb NOT NULL DEFAULT '[]', favorite boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX voice_actors_name_unique ON voice_actors(lower(name));
CREATE TABLE scenes_voice_actors (scene_id integer NOT NULL REFERENCES scenes(id) ON DELETE CASCADE, voice_actor_id integer NOT NULL REFERENCES voice_actors(id) ON DELETE CASCADE, PRIMARY KEY(scene_id,voice_actor_id));
CREATE INDEX scenes_voice_actors_actor_idx ON scenes_voice_actors(voice_actor_id);
