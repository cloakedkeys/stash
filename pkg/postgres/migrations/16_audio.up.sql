--------------------------------------------
-- audios definition
--
CREATE TABLE "audios" (
    "id" serial not null primary key,
    "title" varchar(255),
    "details" text,
    "date" date,
    "date_precision" smallint,
    "studio_id" integer,
    "organized" boolean not null default false,
    "code" text,
    "cover_blob" varchar(255),
    "created_at" timestamptz not null,
    "updated_at" timestamptz not null,
    foreign key("studio_id") references "studios"("id") on delete SET NULL,
    foreign key("cover_blob") references "blobs"("checksum") on delete SET NULL
);
CREATE INDEX "index_audios_on_studio_id" on "audios" ("studio_id");
--------------------------------------------
-- Per-user ratings and playback activity.
CREATE TABLE audios_user_data (
    audio_id integer NOT NULL REFERENCES audios(id) ON DELETE CASCADE,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating smallint,
    resume_time double precision NOT NULL DEFAULT 0,
    play_duration double precision NOT NULL DEFAULT 0,
    PRIMARY KEY (audio_id, user_id)
);
CREATE INDEX index_audios_user_data_on_user_id ON audios_user_data(user_id);
--------------------------------------------
-- audios_o_dates definition
--
CREATE TABLE "audios_o_dates" (
    "audio_id" integer not null,
    "user_id" integer not null REFERENCES users(id) ON DELETE CASCADE,
    "o_date" timestamptz not null,
    foreign key("audio_id") references "audios"("id") on delete CASCADE
);
CREATE INDEX "index_audios_o_dates" ON "audios_o_dates" ("audio_id", "user_id");
CREATE INDEX "index_audios_o_dates_on_user_id" ON "audios_o_dates" ("user_id");
--------------------------------------------
-- audios_tags definition
--
CREATE TABLE "audios_tags" (
    "audio_id" integer,
    "tag_id" integer,
    foreign key("audio_id") references "audios"("id") on delete CASCADE,
    foreign key("tag_id") references "tags"("id") on delete CASCADE,
    PRIMARY KEY("audio_id", "tag_id")
);
CREATE INDEX "index_audios_tags_on_tag_id" on "audios_tags" ("tag_id");
--------------------------------------------
-- audios_play_dates definition
--
CREATE TABLE "audios_play_dates" (
    "audio_id" integer not null,
    "user_id" integer not null REFERENCES users(id) ON DELETE CASCADE,
    "play_date" timestamptz not null,
    foreign key("audio_id") references "audios"("id") on delete CASCADE
);
CREATE INDEX "index_audios_play_dates" ON "audios_play_dates" ("audio_id", "user_id");
CREATE INDEX "index_audios_play_dates_on_user_id" ON "audios_play_dates" ("user_id");
--------------------------------------------
-- groups_audios definition
--
CREATE TABLE "groups_audios" (
    "group_id" integer,
    "audio_id" integer,
    "audio_index" smallint,
    foreign key("group_id") references "groups"("id") on delete cascade,
    foreign key("audio_id") references "audios"("id") on delete cascade,
    PRIMARY KEY("group_id", "audio_id")
);
CREATE INDEX "index_groups_audios_on_audio_id" on "groups_audios" ("audio_id");
--------------------------------------------
-- audios_galleries definition
--
CREATE TABLE "audios_galleries" (
    "audio_id" integer NOT NULL,
    "gallery_id" integer NOT NULL,
    foreign key("audio_id") references "audios"("id") on delete CASCADE,
    foreign key("gallery_id") references "galleries"("id") on delete CASCADE,
    PRIMARY KEY("audio_id", "gallery_id")
);
CREATE INDEX "index_audios_galleries_on_gallery_id" ON "audios_galleries" ("gallery_id");
--------------------------------------------
-- performers_audios definition
--
CREATE TABLE "performers_audios" (
    "performer_id" integer,
    "audio_id" integer,
    foreign key("performer_id") references "performers"("id") on delete CASCADE,
    foreign key("audio_id") references "audios"("id") on delete CASCADE,
    PRIMARY KEY ("audio_id", "performer_id")
);
CREATE INDEX "index_performers_audios_on_performer_id" on "performers_audios" ("performer_id");
--------------------------------------------
-- audio_custom_fields definition
--
CREATE TABLE "audio_custom_fields" (
    "audio_id" integer NOT NULL,
    "field" varchar(64) NOT NULL,
    "value" jsonb NOT NULL, "type" text,
    PRIMARY KEY ("audio_id", "field"),
    foreign key("audio_id") references "audios"("id") on delete CASCADE
);
CREATE INDEX "index_audio_custom_fields_field_value" ON "audio_custom_fields" ("field", "value");
--------------------------------------------
-- audio_urls definition
--
CREATE TABLE "audio_urls" (
    "audio_id" integer NOT NULL,
    "position" integer NOT NULL,
    "url" varchar(255) NOT NULL,
    foreign key("audio_id") references "audios"("id") on delete CASCADE,
    PRIMARY KEY("audio_id", "position", "url")
);
CREATE INDEX "audio_urls_url" on "audio_urls" ("url");
--------------------------------------------
-- audios_files definition
--
CREATE TABLE "audios_files" (
    "audio_id" integer NOT NULL,
    "file_id" integer NOT NULL,
    "primary" boolean NOT NULL,
    foreign key("audio_id") references "audios"("id") on delete CASCADE,
    foreign key("file_id") references "files"("id") on delete CASCADE,
    PRIMARY KEY("audio_id", "file_id")
);
CREATE INDEX "index_audios_files_file_id" ON "audios_files" ("file_id");
CREATE UNIQUE INDEX "unique_index_audios_files_on_primary" on "audios_files" ("audio_id")
WHERE "primary" = true;
--------------------------------------------
-- audio_files definition
--

CREATE TABLE "audio_files" (
    "file_id" integer NOT NULL primary key,
    "duration" double precision NOT NULL,
    "format" varchar(255) NOT NULL,
    "audio_codec" varchar(255) NOT NULL,
    "sample_rate" integer NOT NULL,
    "bit_rate" integer NOT NULL,
    foreign key("file_id") references "files"("id") on delete CASCADE
);
