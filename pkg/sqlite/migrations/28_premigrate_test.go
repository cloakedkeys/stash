//go:build integration

package migrations

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/require"
)

// upgradeTo upgrades the database to version, as the upgrade task does.
func upgradeTo(t *testing.T, path string, version uint) error {
	t.Helper()
	ctx := context.Background()
	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	defer db.Close()
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for m.CurrentSchemaVersion() < version {
		v := m.CurrentSchemaVersion() + 1
		if err := m.RunMigration(ctx, v); err != nil {
			return fmt.Errorf("schema %d: %w", v, err)
		}
	}
	return nil
}

// Images with the same path don't stop the upgrade: they are merged into the
// one with the lowest id, which gets all their links, fills its empty fields
// from them, lowest id first, and adds up their o-counters (#2244).
func TestPre28MergesSamePathImages(t *testing.T) {
	path := databaseAt(t, 27, `
INSERT INTO studios(id,name,checksum,created_at,updated_at) VALUES
  (1,'S1','s1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'S2','s2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO tags(id,name,created_at,updated_at) VALUES
  (1,'T1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'T2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'P1','p1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'P2','p2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO galleries(id,path,checksum,created_at,updated_at) VALUES
  (1,'/g1','g1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'/g2','g2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO images(id,path,checksum,title,rating,size,width,height,studio_id,o_counter,file_mod_time,organized,created_at,updated_at) VALUES
  (1,'/a.jpg','c1','',  NULL,NULL,640, NULL,NULL,1,NULL,                 0,'2020-01-01T00:00:00Z','2020-01-01T00:00:00Z'),
  (2,'/a.jpg','c2','Two',3,   NULL,1024,480, 1,   2,NULL,                 1,'2020-01-02T00:00:00Z','2020-01-02T00:00:00Z'),
  (3,'/a.jpg','c3','Three',5, 100, 800, 600, 2,   4,'2020-01-03T00:00:00Z',0,'2020-01-03T00:00:00Z','2020-01-03T00:00:00Z'),
  (4,'/b.jpg','c4','Other',1, 50,  10,  10,  NULL,0,NULL,                 0,'2020-01-04T00:00:00Z','2020-01-04T00:00:00Z');
INSERT INTO images_tags(image_id,tag_id) VALUES (1,1),(2,1),(2,2),(4,2);
INSERT INTO performers_images(image_id,performer_id) VALUES (1,1),(2,2),(3,2);
INSERT INTO galleries_images(gallery_id,image_id) VALUES (1,1),(2,3),(1,3);`)
	log := captureWarnings(t)

	require.NoError(t, upgradeTo(t, path, 28))

	require.Equal(t, []string{
		// image 1's own path, checksum, width and dates; the title, rating
		// and studio from 2; the size and file time from 3; organized from
		// 2; o-counter 1 + 2 + 4
		"1|/a.jpg|c1|Two|3|100|640|480|1|7|2020-01-03T00:00:00Z|1|2020-01-01T00:00:00Z",
		"4|/b.jpg|c4|Other|1|50|10|10||0||0|2020-01-04T00:00:00Z",
	}, queryStrings(t, path, `SELECT id || '|' || path || '|' || checksum || '|' || title || '|' || rating || '|' || size || '|' || width || '|' || height || '|' || COALESCE(studio_id, '') || '|' || o_counter || '|' || COALESCE(file_mod_time, '') || '|' || organized || '|' || created_at FROM images ORDER BY id`))
	require.Equal(t, []string{"1 1", "1 2", "4 2"}, queryStrings(t, path, "SELECT image_id || ' ' || tag_id FROM images_tags ORDER BY image_id, tag_id"))
	require.Equal(t, []string{"1 1", "1 2"}, queryStrings(t, path, "SELECT image_id || ' ' || performer_id FROM performers_images ORDER BY image_id, performer_id"))
	require.Equal(t, []string{"1 1", "1 2"}, queryStrings(t, path, "SELECT image_id || ' ' || gallery_id FROM galleries_images ORDER BY image_id, gallery_id"))

	var merges []string
	for _, w := range log.warnings {
		if strings.HasPrefix(w, "Images with the path") {
			merges = append(merges, w)
		}
	}
	require.Equal(t, []string{`Images with the path "/a.jpg", which must now be unique: keeping image 1 and merging images [2 3] into it.`}, merges)

	// and the rest of the upgrade finishes
	require.NoError(t, upgradeToLatest(t, path))
}

// Without images that share a path, the images don't change.
func TestPre28KeepsImages(t *testing.T) {
	path := databaseAt(t, 27, `
INSERT INTO images(id,path,checksum,title,o_counter,created_at,updated_at) VALUES
  (1,'/a.jpg','c1','',0,'2020-01-01T00:00:00Z','2020-01-01T00:00:00Z'),
  (2,'/b.jpg','c2',NULL,3,'2020-01-02T00:00:00Z','2020-01-02T00:00:00Z');
INSERT INTO images_tags(image_id,tag_id) VALUES (1,1),(2,1);`)

	require.NoError(t, upgradeTo(t, path, 28))

	require.Equal(t, []string{"1|/a.jpg||0", "2|/b.jpg|NULL|3"},
		queryStrings(t, path, "SELECT id || '|' || path || '|' || COALESCE(title, 'NULL') || '|' || o_counter FROM images ORDER BY id"))
	require.Equal(t, []string{"1 1", "2 1"}, queryStrings(t, path, "SELECT image_id || ' ' || tag_id FROM images_tags ORDER BY image_id"))
}
