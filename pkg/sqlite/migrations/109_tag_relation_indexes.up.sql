-- #2196: finding a tag's parents, and deleting a tag, scanned the whole
-- tags_relations table: its primary key starts with parent_id. Deleting a tag
-- also scanned tag_stash_ids, which had no index at all. IF NOT EXISTS: a
-- database where someone already added one of these still upgrades.
CREATE INDEX IF NOT EXISTS `index_tags_relations_on_child_id` ON `tags_relations` (`child_id`, `parent_id`);
CREATE INDEX IF NOT EXISTS `index_tag_stash_ids_on_tag_id` ON `tag_stash_ids` (`tag_id`);
