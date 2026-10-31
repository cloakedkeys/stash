-- add an o-counter to galleries, stored the same way as on images.
-- existing galleries start at 0.
ALTER TABLE `galleries` ADD COLUMN `o_counter` tinyint not null default 0;
