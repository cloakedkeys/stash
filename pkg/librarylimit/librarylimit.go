// Package librarylimit works out what an account with a library limit may see.
//
// It builds SQL that is valid on both database backends. The only values placed in
// the SQL are integers, so nothing a user provides can change its meaning.
package librarylimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/stashapp/stash/pkg/models"
)

// Kind is a type of object in the library.
type Kind int

const (
	Scenes Kind = iota
	Images
	Galleries
	Audios
	SceneMarkers
	Performers
	Tags
	Studios
	Groups
	VoiceActors
)

var tables = map[Kind]string{
	Scenes:       "scenes",
	Images:       "images",
	Galleries:    "galleries",
	Audios:       "audios",
	SceneMarkers: "scene_markers",
	Performers:   "performers",
	Tags:         "tags",
	Studios:      "studios",
	Groups:       "groups",
	VoiceActors:  "voice_actors",
}

const nothing = "SELECT 0 WHERE 1 = 0"

// Where returns the condition an object of this kind in its own table must meet.
// It returns false if ctx has no limit.
func Where(ctx context.Context, kind Kind) (string, bool) {
	l := models.LibraryLimitFromContext(ctx)
	if l == nil {
		return "", false
	}

	return fmt.Sprintf(`"%s"."id" IN (%s)`, tables[kind], visible(l, kind)), true
}

// Apply limits a query that selects from the kind's table.
func Apply(ctx context.Context, q *goqu.SelectDataset, kind Kind) *goqu.SelectDataset {
	if where, ok := Where(ctx, kind); ok {
		return q.Where(goqu.L(where))
	}

	return q
}

// WrapQuery limits a query written in SQL, whose rows have an id column.
func WrapQuery(ctx context.Context, query string, kind Kind) string {
	l := models.LibraryLimitFromContext(ctx)
	if l == nil {
		return query
	}

	return fmt.Sprintf(`SELECT * FROM (%s) AS "limited" WHERE "limited"."id" IN (%s)`, query, visible(l, kind))
}

// IDsQuery returns a query for which of the given IDs may be seen, or "" if there are no IDs.
func IDsQuery(l *models.LibraryLimit, kind Kind, ids []int) string {
	if len(ids) == 0 {
		return ""
	}

	return fmt.Sprintf(`SELECT "id" FROM "%s" WHERE "id" IN (%s) AND "id" IN (%s)`, tables[kind], intList(ids), visible(l, kind))
}

// KeepVisible returns the IDs in found, in the order of ids.
func KeepVisible(ids, found []int) []int {
	seen := make(map[int]bool, len(found))
	for _, id := range found {
		seen[id] = true
	}

	ret := make([]int, 0, len(found))
	for _, id := range ids {
		if seen[id] {
			ret = append(ret, id)
		}
	}

	return ret
}

func intList(ids []int) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}

	return strings.Join(s, ",")
}

func union(parts ...string) string {
	var ret []string
	for _, p := range parts {
		if p != "" {
			ret = append(ret, p)
		}
	}

	if len(ret) == 0 {
		return nothing
	}

	return strings.Join(ret, " UNION ")
}

func allowedFolders(l *models.LibraryLimit) string {
	return fmt.Sprintf(`WITH RECURSIVE "allowed_folders"("id") AS (SELECT "id" FROM "folders" WHERE "id" IN (%s) UNION SELECT "c"."id" FROM "folders" "c" JOIN "allowed_folders" "a" ON "c"."parent_folder_id" = "a"."id") SELECT "id" FROM "allowed_folders"`, intList(l.FolderIDs))
}

func allowedTags(l *models.LibraryLimit) string {
	return fmt.Sprintf(`WITH RECURSIVE "allowed_tags"("id") AS (SELECT "id" FROM "tags" WHERE "id" IN (%s) UNION SELECT "r"."child_id" FROM "tags_relations" "r" JOIN "allowed_tags" "a" ON "r"."parent_id" = "a"."id") SELECT "id" FROM "allowed_tags"`, intList(l.TagIDs))
}

func allowedStudios(l *models.LibraryLimit) string {
	return fmt.Sprintf(`WITH RECURSIVE "allowed_studios"("id") AS (SELECT "id" FROM "studios" WHERE "id" IN (%s) UNION SELECT "c"."id" FROM "studios" "c" JOIN "allowed_studios" "a" ON "c"."parent_id" = "a"."id") SELECT "id" FROM "allowed_studios"`, intList(l.StudioIDs))
}

// content returns the IDs in a table of scenes, images, galleries or audios that match the limit directly.
func content(l *models.LibraryLimit, table, idColumn, filesTable, tagsTable string) string {
	var inFolder, tagged, inStudio string

	if len(l.FolderIDs) > 0 {
		inFolder = fmt.Sprintf(`SELECT "j"."%[2]s" FROM "%[1]s" "j" JOIN "files" "f" ON "f"."id" = "j"."file_id" WHERE "f"."parent_folder_id" IN (%[3]s)`, filesTable, idColumn, allowedFolders(l))
	}
	if len(l.TagIDs) > 0 {
		tagged = fmt.Sprintf(`SELECT "%s" FROM "%s" WHERE "tag_id" IN (%s)`, idColumn, tagsTable, allowedTags(l))
	}
	if len(l.StudioIDs) > 0 {
		inStudio = fmt.Sprintf(`SELECT "id" FROM "%s" WHERE "studio_id" IN (%s)`, table, allowedStudios(l))
	}

	return union(inFolder, tagged, inStudio)
}

func visibleScenes(l *models.LibraryLimit) string {
	return content(l, "scenes", "scene_id", "scenes_files", "scenes_tags")
}

func visibleAudios(l *models.LibraryLimit) string {
	return content(l, "audios", "audio_id", "audios_files", "audios_tags")
}

func visibleGalleries(l *models.LibraryLimit) string {
	folderGalleries := ""
	if len(l.FolderIDs) > 0 {
		folderGalleries = fmt.Sprintf(`SELECT "id" FROM "galleries" WHERE "folder_id" IN (%s)`, allowedFolders(l))
	}

	return union(content(l, "galleries", "gallery_id", "galleries_files", "galleries_tags"), folderGalleries)
}

// visibleImages includes the images of galleries that may be seen, or those galleries would look empty.
func visibleImages(l *models.LibraryLimit) string {
	return union(
		content(l, "images", "image_id", "images_files", "images_tags"),
		fmt.Sprintf(`SELECT "image_id" FROM "galleries_images" WHERE "gallery_id" IN (%s)`, visibleGalleries(l)),
	)
}

func visibleSceneMarkers(l *models.LibraryLimit) string {
	return fmt.Sprintf(`SELECT "id" FROM "scene_markers" WHERE "scene_id" IN (%s)`, visibleScenes(l))
}

// A performer, tag, studio, group or voice actor may be seen if something that may be seen
// refers to it. Otherwise its name and counts would reveal what is hidden.
func visiblePerformers(l *models.LibraryLimit) string {
	return union(
		fmt.Sprintf(`SELECT "performer_id" FROM "performers_scenes" WHERE "scene_id" IN (%s)`, visibleScenes(l)),
		fmt.Sprintf(`SELECT "performer_id" FROM "performers_images" WHERE "image_id" IN (%s)`, visibleImages(l)),
		fmt.Sprintf(`SELECT "performer_id" FROM "performers_galleries" WHERE "gallery_id" IN (%s)`, visibleGalleries(l)),
		fmt.Sprintf(`SELECT "performer_id" FROM "performers_audios" WHERE "audio_id" IN (%s)`, visibleAudios(l)),
	)
}

func visibleGroups(l *models.LibraryLimit) string {
	return union(
		fmt.Sprintf(`SELECT "group_id" FROM "groups_scenes" WHERE "scene_id" IN (%s)`, visibleScenes(l)),
		fmt.Sprintf(`SELECT "group_id" FROM "groups_audios" WHERE "audio_id" IN (%s)`, visibleAudios(l)),
	)
}

func visibleStudios(l *models.LibraryLimit) string {
	allowed := ""
	if len(l.StudioIDs) > 0 {
		allowed = allowedStudios(l)
	}

	return union(
		allowed,
		fmt.Sprintf(`SELECT "studio_id" FROM "scenes" WHERE "id" IN (%s)`, visibleScenes(l)),
		fmt.Sprintf(`SELECT "studio_id" FROM "images" WHERE "id" IN (%s)`, visibleImages(l)),
		fmt.Sprintf(`SELECT "studio_id" FROM "galleries" WHERE "id" IN (%s)`, visibleGalleries(l)),
		fmt.Sprintf(`SELECT "studio_id" FROM "audios" WHERE "id" IN (%s)`, visibleAudios(l)),
		fmt.Sprintf(`SELECT "studio_id" FROM "groups" WHERE "id" IN (%s)`, visibleGroups(l)),
	)
}

func visibleTags(l *models.LibraryLimit) string {
	allowed := ""
	if len(l.TagIDs) > 0 {
		allowed = allowedTags(l)
	}

	return union(
		allowed,
		fmt.Sprintf(`SELECT "tag_id" FROM "scenes_tags" WHERE "scene_id" IN (%s)`, visibleScenes(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "images_tags" WHERE "image_id" IN (%s)`, visibleImages(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "galleries_tags" WHERE "gallery_id" IN (%s)`, visibleGalleries(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "audios_tags" WHERE "audio_id" IN (%s)`, visibleAudios(l)),
		fmt.Sprintf(`SELECT "primary_tag_id" FROM "scene_markers" WHERE "id" IN (%s)`, visibleSceneMarkers(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "scene_markers_tags" WHERE "scene_marker_id" IN (%s)`, visibleSceneMarkers(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "performers_tags" WHERE "performer_id" IN (%s)`, visiblePerformers(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "studios_tags" WHERE "studio_id" IN (%s)`, visibleStudios(l)),
		fmt.Sprintf(`SELECT "tag_id" FROM "groups_tags" WHERE "group_id" IN (%s)`, visibleGroups(l)),
	)
}

func visibleVoiceActors(l *models.LibraryLimit) string {
	return fmt.Sprintf(`SELECT "voice_actor_id" FROM "scenes_voice_actors" WHERE "scene_id" IN (%s)`, visibleScenes(l))
}

// visible fails closed: a kind it doesn't know allows nothing.
func visible(l *models.LibraryLimit, kind Kind) string {
	switch kind {
	case Scenes:
		return visibleScenes(l)
	case Images:
		return visibleImages(l)
	case Galleries:
		return visibleGalleries(l)
	case Audios:
		return visibleAudios(l)
	case SceneMarkers:
		return visibleSceneMarkers(l)
	case Performers:
		return visiblePerformers(l)
	case Tags:
		return visibleTags(l)
	case Studios:
		return visibleStudios(l)
	case Groups:
		return visibleGroups(l)
	case VoiceActors:
		return visibleVoiceActors(l)
	}

	return nothing
}
