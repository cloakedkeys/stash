package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stashapp/stash/pkg/hash/md5"
	"github.com/stashapp/stash/pkg/models"
)

// GetAlternateImages loads only references, never blob data.
func (qb *PerformerStore) GetAlternateImages(ctx context.Context, performerID int) ([]models.PerformerImage, error) {
	ret := []models.PerformerImage{}
	err := dbWrapper.Select(ctx, &ret, "SELECT image_blob, position FROM performer_images WHERE performer_id = ? ORDER BY position, image_blob", performerID)
	return ret, err
}

func (qb *PerformerStore) GetAlternateImage(ctx context.Context, performerID int, checksum string) ([]byte, error) {
	var found string
	if err := dbWrapper.Get(ctx, &found, "SELECT image_blob FROM performer_images WHERE performer_id = ? AND image_blob = ?", performerID, checksum); err != nil {
		return nil, err
	}
	return qb.blobStore.Read(ctx, found)
}

// Serialize reference changes for one performer, including appending positions.
func (qb *PerformerStore) lockImageReferences(ctx context.Context, performerID int) (*string, error) {
	var primary sql.NullString
	if err := dbWrapper.Get(ctx, &primary, "SELECT image_blob FROM performers WHERE id = ?", performerID); err != nil {
		return nil, err
	}
	if !primary.Valid {
		return nil, nil
	}
	return &primary.String, nil
}

func (qb *PerformerStore) AddAlternateImage(ctx context.Context, performerID int, image []byte) (bool, error) {
	primary, err := qb.lockImageReferences(ctx, performerID)
	if err != nil {
		return false, err
	}
	checksum := md5.FromBytes(image)
	if primary != nil && *primary == checksum {
		return false, nil
	}
	var count int
	if err := dbWrapper.Get(ctx, &count, "SELECT COUNT(*) FROM performer_images WHERE performer_id = ? AND image_blob = ?", performerID, checksum); err != nil {
		return false, err
	}
	if count != 0 {
		return false, nil
	}
	checksum, err = qb.blobStore.Write(ctx, image)
	if err != nil {
		return false, err
	}
	return qb.appendAlternateImage(ctx, performerID, checksum)
}

// appendAlternateImage attaches an existing blob. The caller holds the performer lock.
func (qb *PerformerStore) appendAlternateImage(ctx context.Context, performerID int, checksum string) (bool, error) {
	result, err := dbWrapper.Exec(ctx, `INSERT INTO performer_images (performer_id, image_blob, position)
SELECT ?, ?, COALESCE(MAX(position), -1) + 1 FROM performer_images WHERE performer_id = ?
ON CONFLICT (performer_id, image_blob) DO NOTHING`, performerID, checksum, performerID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n != 0, err
}

func (qb *PerformerStore) RemoveAlternateImage(ctx context.Context, performerID int, checksum string) error {
	if _, err := qb.lockImageReferences(ctx, performerID); err != nil {
		return err
	}
	result, err := dbWrapper.Exec(ctx, "DELETE FROM performer_images WHERE performer_id = ? AND image_blob = ?", performerID, checksum)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return qb.blobStore.Delete(ctx, checksum)
}

func (qb *PerformerStore) SetPrimaryImage(ctx context.Context, performerID int, checksum string) error {
	primary, err := qb.lockImageReferences(ctx, performerID)
	if err != nil {
		return err
	}
	var position int
	if err := dbWrapper.Get(ctx, &position, "SELECT position FROM performer_images WHERE performer_id = ? AND image_blob = ?", performerID, checksum); err != nil {
		return err
	}
	if _, err := dbWrapper.Exec(ctx, "UPDATE performers SET image_blob = ? WHERE id = ?", checksum, performerID); err != nil {
		return err
	}
	if _, err := dbWrapper.Exec(ctx, "DELETE FROM performer_images WHERE performer_id = ? AND image_blob = ?", performerID, checksum); err != nil {
		return err
	}
	if primary != nil && *primary != checksum {
		_, err = dbWrapper.Exec(ctx, `INSERT INTO performer_images (performer_id, image_blob, position) VALUES (?, ?, ?)
ON CONFLICT (performer_id, image_blob) DO NOTHING`, performerID, *primary, position)
	}
	return err
}

func (qb *PerformerStore) ReorderAlternateImages(ctx context.Context, performerID int, checksums []string) error {
	if _, err := qb.lockImageReferences(ctx, performerID); err != nil {
		return err
	}
	images, err := qb.GetAlternateImages(ctx, performerID)
	if err != nil {
		return err
	}
	remaining := make(map[string]bool, len(images))
	for _, image := range images {
		remaining[image.Checksum] = true
	}
	if len(images) != len(checksums) {
		return fmt.Errorf("reorder must include every alternate image exactly once")
	}
	for _, checksum := range checksums {
		if !remaining[checksum] {
			return fmt.Errorf("reorder contains a duplicate or unknown alternate image %q", checksum)
		}
		delete(remaining, checksum)
	}
	for position, checksum := range checksums {
		if _, err := dbWrapper.Exec(ctx, "UPDATE performer_images SET position = ? WHERE performer_id = ? AND image_blob = ?", position, performerID, checksum); err != nil {
			return err
		}
	}
	return nil
}

func (qb *PerformerStore) mergeImageReferences(ctx context.Context, source []int, destination int) error {
	primary, err := qb.lockImageReferences(ctx, destination)
	if err != nil {
		return err
	}
	for _, id := range source {
		sourcePrimary, err := qb.lockImageReferences(ctx, id)
		if err != nil {
			return err
		}
		images, err := qb.GetAlternateImages(ctx, id)
		if err != nil {
			return err
		}
		if sourcePrimary != nil {
			if primary == nil {
				if _, err := dbWrapper.Exec(ctx, "UPDATE performers SET image_blob = ? WHERE id = ?", *sourcePrimary, destination); err != nil {
					return err
				}
				if _, err := dbWrapper.Exec(ctx, "DELETE FROM performer_images WHERE performer_id = ? AND image_blob = ?", destination, *sourcePrimary); err != nil {
					return err
				}
				primary = sourcePrimary
			} else {
				images = append([]models.PerformerImage{{Checksum: *sourcePrimary}}, images...)
			}
		}
		for _, image := range images {
			if primary != nil && *primary == image.Checksum {
				continue
			}
			if _, err := qb.appendAlternateImage(ctx, destination, image.Checksum); err != nil {
				return err
			}
		}
	}
	return nil
}
