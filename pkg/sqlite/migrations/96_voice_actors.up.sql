CREATE TABLE voice_actors (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK (length(trim(name)) > 0),
    details TEXT NOT NULL DEFAULT '',
    aliases TEXT NOT NULL DEFAULT '[]',
    urls TEXT NOT NULL DEFAULT '[]',
    favorite BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE scenes_voice_actors (
    scene_id INTEGER NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
    voice_actor_id INTEGER NOT NULL REFERENCES voice_actors(id) ON DELETE CASCADE,
    PRIMARY KEY (scene_id, voice_actor_id)
);
CREATE INDEX scenes_voice_actors_actor_idx ON scenes_voice_actors(voice_actor_id);
