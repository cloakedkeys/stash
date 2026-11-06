package migrations

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
)

// pre28 merges images that have the same path. Migration 28 makes the path
// unique, so the upgrade would otherwise stop (#2244).
//
// The image with the lowest id is kept. It gets the tags, performers and
// galleries of the others, and any of its fields that are empty are filled
// from them, lowest id first. The o-counters are added together, and it
// counts as organized if any of them was. Then the others are deleted.
func pre28(ctx context.Context, db *sqlx.DB) error {
	m := migrator{db: db}
	return m.withTxn(ctx, func(tx *sqlx.Tx) error {
		var paths []string
		if err := tx.Select(&paths, "SELECT path FROM images GROUP BY path HAVING COUNT(*) > 1 ORDER BY path"); err != nil {
			return err
		}
		for _, path := range paths {
			if err := mergeSamePathImages(tx, path); err != nil {
				return fmt.Errorf("merging images with path %q: %w", path, err)
			}
		}
		return nil
	})
}

// imageLinks are the tables that link images to other objects at schema 27,
// with the column of the other object.
var imageLinks = []struct{ table, column string }{
	{"performers_images", "performer_id"},
	{"images_tags", "tag_id"},
	{"galleries_images", "gallery_id"},
}

func mergeSamePathImages(tx *sqlx.Tx, path string) error {
	var ids []int
	if err := tx.Select(&ids, "SELECT id FROM images WHERE path = ? ORDER BY id", path); err != nil {
		return err
	}
	keep, remove := ids[0], ids[1:]
	logger.Warnf("Images with the path %q, which must now be unique: keeping image %d and merging images %v into it.", path, keep, remove)

	for _, id := range remove {
		// fill what the kept image doesn't have; an empty title counts as
		// missing
		if _, err := tx.Exec(`
UPDATE images SET
  title = COALESCE(NULLIF(images.title, ''), NULLIF(d.title, ''), images.title),
  rating = COALESCE(images.rating, d.rating),
  size = COALESCE(images.size, d.size),
  width = COALESCE(images.width, d.width),
  height = COALESCE(images.height, d.height),
  studio_id = COALESCE(images.studio_id, d.studio_id),
  file_mod_time = COALESCE(images.file_mod_time, d.file_mod_time),
  o_counter = images.o_counter + d.o_counter,
  organized = MAX(images.organized, d.organized)
FROM images AS d
WHERE images.id = ? AND d.id = ?`, keep, id); err != nil {
			return err
		}

		// the link tables have no unique index, so links the kept image
		// already has are left out by hand
		for _, link := range imageLinks {
			if _, err := tx.Exec(fmt.Sprintf(`
INSERT INTO %[1]s (image_id, %[2]s)
SELECT DISTINCT ?, %[2]s FROM %[1]s WHERE image_id = ? AND %[2]s NOT IN (
  SELECT %[2]s FROM %[1]s WHERE image_id = ? AND %[2]s IS NOT NULL
)`, link.table, link.column), keep, id, keep); err != nil {
				return err
			}
			// foreign keys are off during upgrades, so nothing cascades
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE image_id = ?", link.table), id); err != nil {
				return err
			}
		}

		if _, err := tx.Exec("DELETE FROM images WHERE id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	sqlite.RegisterPreMigration(28, pre28)
}
