-- #2192: the tag indexes of the tag join tables also hold the item id, so
-- counting or finding the items of a tag reads the index only, not every
-- table row. Each index keeps its name, so it still starts with tag_id and
-- still serves everything the single-column index did. A database that lost
-- one of the old indexes, for example to a repair tool, still upgrades.
DROP INDEX IF EXISTS `index_scenes_tags_on_tag_id`;
CREATE INDEX `index_scenes_tags_on_tag_id` ON `scenes_tags` (`tag_id`, `scene_id`);
DROP INDEX IF EXISTS `index_images_tags_on_tag_id`;
CREATE INDEX `index_images_tags_on_tag_id` ON `images_tags` (`tag_id`, `image_id`);
DROP INDEX IF EXISTS `index_galleries_tags_on_tag_id`;
CREATE INDEX `index_galleries_tags_on_tag_id` ON `galleries_tags` (`tag_id`, `gallery_id`);
DROP INDEX IF EXISTS `index_performers_tags_on_tag_id`;
CREATE INDEX `index_performers_tags_on_tag_id` ON `performers_tags` (`tag_id`, `performer_id`);
DROP INDEX IF EXISTS `index_studios_tags_on_tag_id`;
CREATE INDEX `index_studios_tags_on_tag_id` ON `studios_tags` (`tag_id`, `studio_id`);
DROP INDEX IF EXISTS `index_groups_tags_on_tag_id`;
CREATE INDEX `index_groups_tags_on_tag_id` ON `groups_tags` (`tag_id`, `group_id`);
DROP INDEX IF EXISTS `index_audios_tags_on_tag_id`;
CREATE INDEX `index_audios_tags_on_tag_id` ON `audios_tags` (`tag_id`, `audio_id`);
DROP INDEX IF EXISTS `index_stories_tags_on_tag_id`;
CREATE INDEX `index_stories_tags_on_tag_id` ON `stories_tags` (`tag_id`, `story_id`);
DROP INDEX IF EXISTS `index_scene_markers_tags_on_tag_id`;
CREATE INDEX `index_scene_markers_tags_on_tag_id` ON `scene_markers_tags` (`tag_id`, `scene_marker_id`);
