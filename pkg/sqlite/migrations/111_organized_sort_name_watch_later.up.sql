ALTER TABLE `performers` ADD COLUMN `organized` boolean NOT NULL DEFAULT '0';
ALTER TABLE `tags` ADD COLUMN `organized` boolean NOT NULL DEFAULT '0';
ALTER TABLE `performers` ADD COLUMN `sort_name` text;
ALTER TABLE `scenes_user_data` ADD COLUMN `watch_later_at` datetime;
