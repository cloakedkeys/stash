-- Where a scene starts and ends in its file, in seconds. NULL is the start or end of the file.
ALTER TABLE `scenes_files` ADD COLUMN `start_time` REAL;
ALTER TABLE `scenes_files` ADD COLUMN `end_time` REAL;
