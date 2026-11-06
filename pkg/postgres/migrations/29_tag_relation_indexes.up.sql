-- #2196: finding a tag's parents, and deleting a tag, scanned the whole
-- tags_relations table: its primary key starts with parent_id. IF NOT EXISTS:
-- a database where someone already added it still upgrades.
--
-- tag_stash_ids needs nothing here: tag_stash_ids_unique_idx starts with
-- tag_id (migration 12).
CREATE INDEX IF NOT EXISTS index_tags_relations_on_child_id ON tags_relations (child_id, parent_id);
