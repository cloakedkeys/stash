CREATE TABLE "stories" (
    `id` integer not null primary key autoincrement,
    `title` varchar(255),
    `details` text,
    `author` text,
    `source_website` text,
    `language` text,
    `tagline` text,
    `date` date,
    "date_precision" TINYINT,
    `created_at` datetime not null,
    `updated_at` datetime not null
);

CREATE TABLE `stories_user_data` (
    `story_id` integer NOT NULL,
    `user_id` integer NOT NULL,
    `rating` tinyint,
    PRIMARY KEY (`story_id`, `user_id`),
    FOREIGN KEY (`story_id`) REFERENCES `stories`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE
);
CREATE INDEX `index_stories_user_data_on_user_id` ON `stories_user_data` (`user_id`);

CREATE TABLE "stories_tags" (
    `story_id` integer,
    `tag_id` integer,
    foreign key(`story_id`) references `stories`(`id`) on delete CASCADE,
    foreign key(`tag_id`) references `tags`(`id`) on delete CASCADE,
    PRIMARY KEY(`story_id`, `tag_id`)
);
CREATE INDEX `index_stories_tags_on_tag_id` on `stories_tags` (`tag_id`);

CREATE TABLE `story_urls` (
    `story_id` integer NOT NULL,
    `position` integer NOT NULL,
    `url` varchar(255) NOT NULL,
    foreign key(`story_id`) references `stories`(`id`) on delete CASCADE,
    PRIMARY KEY(`story_id`, `position`, `url`)
);
CREATE INDEX `story_urls_url` on `story_urls` (`url`);

CREATE TABLE `stories_files` (
    `story_id` integer NOT NULL,
    `file_id` integer NOT NULL,
    `primary` boolean NOT NULL,
    foreign key(`story_id`) references `stories`(`id`) on delete CASCADE,
    foreign key(`file_id`) references `files`(`id`) on delete CASCADE,
    PRIMARY KEY(`story_id`, `file_id`)
);
CREATE INDEX `index_stories_files_file_id` ON `stories_files` (`file_id`);
CREATE UNIQUE INDEX `unique_index_stories_files_on_primary` on `stories_files` (`story_id`)
WHERE `primary` = 1;
