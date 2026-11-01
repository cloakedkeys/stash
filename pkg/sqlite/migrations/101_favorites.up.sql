ALTER TABLE scenes_user_data ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE images_user_data ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE galleries_user_data ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE groups_user_data ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE audios_user_data ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT false;
