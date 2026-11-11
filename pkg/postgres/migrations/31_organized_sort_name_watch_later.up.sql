ALTER TABLE performers ADD COLUMN organized boolean NOT NULL DEFAULT false;
ALTER TABLE tags ADD COLUMN organized boolean NOT NULL DEFAULT false;
ALTER TABLE performers ADD COLUMN sort_name text;
ALTER TABLE scenes_user_data ADD COLUMN watch_later_at timestamp with time zone;
