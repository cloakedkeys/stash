//go:build integration

package api

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/audio"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/gallery"
	"github.com/stashapp/stash/pkg/group"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/postgres"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"

	// registers the SQLite post-migrations
	_ "github.com/stashapp/stash/pkg/sqlite/migrations"
)

// openUserDataTestDatabase opens a new SQLite database, or a throwaway PostgreSQL
// database when PGSQL_TEST is set.
func openUserDataTestDatabase(ctx context.Context, t *testing.T) database.Database {
	t.Helper()
	config.InitializeEmpty()

	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		db := sqlite.NewDatabase()
		require.NoError(t, db.Open(ctx, filepath.Join(t.TempDir(), "stash.sqlite")))
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		return db
	}

	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("user_data_missing_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE " + name)
		require.NoError(t, err)
		require.NoError(t, admin.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name

	db := postgres.NewDatabase()
	require.NoError(t, db.Open(ctx, u.String()))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestUserDataUpdateMissingObjectOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "rater", Roles: models.Roles{models.RoleEnumRead}}
	editor := models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	performer := models.NewPerformer()
	performer.Name = "Someone"
	scene := models.Scene{Title: "A scene"}
	img := models.Image{Title: "An image"}
	grp := models.Group{Name: "A group"}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		if err := db.User().Create(ctx, &editor, ""); err != nil {
			return err
		}
		if err := db.Scene().Create(ctx, &scene, nil); err != nil {
			return err
		}
		if err := db.Image().Create(ctx, &models.CreateImageInput{Image: &img}); err != nil {
			return err
		}
		if err := db.Group().Create(ctx, &grp); err != nil {
			return err
		}
		return db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer})
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}, groupService: &group.Service{Repository: db.Group()}}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	// the database's own error, or one naming its table, used to come back for all of these
	check := func(u models.User, mutations map[string]string) {
		for mutation, want := range mutations {
			t.Run(mutation, func(t *testing.T) {
				body := storyRequest(t, gql, u, "mutation { "+mutation+" }")
				require.Contains(t, body, `"message":"`+want+`"`)
				require.NotContains(t, body, "FOREIGN KEY")
				require.NotContains(t, body, "foreign key")
				require.NotContains(t, body, "does not exist")
			})
		}
	}
	check(user, userDataMissingMutations)
	check(editor, missingObjectUpdateMutations)

	// removing or resetting on a missing scene or track gives 0, as upstream does
	for mutation, want := range missingObjectNoOpMutations {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, user, "mutation { "+mutation+" }")
			require.JSONEq(t, want, body)
		})
	}

	// an object that exists is rated as before
	body := storyRequest(t, gql, user, fmt.Sprintf(`mutation { performerUserDataUpdate(input:{id:"%d",rating100:80,favorite:true}) { rating100 favorite } }`, performer.ID))
	require.JSONEq(t, `{"data":{"performerUserDataUpdate":{"rating100":80,"favorite":true}}}`, body)

	// and O and plays are counted as before
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneAddO(id:"%d") { count } }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneAddO":{"count":1}}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneAddPlay(id:"%d") { count } }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneAddPlay":{"count":1}}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneSaveActivity(id:"%d",resume_time:5,playDuration:5) }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneSaveActivity":true}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { imageIncrementO(id:"%d") }`, img.ID))
	require.JSONEq(t, `{"data":{"imageIncrementO":1}}`, body)

	// a group's aliases are still tidied, leaving out its name and repeats
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { groupUpdate(input:{id:"%d",alias_list:["Other","a group","other"]}) { alias_list } }`, grp.ID))
	require.JSONEq(t, `{"data":{"groupUpdate":{"alias_list":["Other"]}}}`, body)
}

// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestMissingParentOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	editor := models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	performer := models.NewPerformer()
	performer.Name = "Someone"
	sc := models.Scene{Title: "A scene"}
	img := models.Image{Title: "An image"}
	gal := models.Gallery{Title: "A gallery"}
	grp := models.Group{Name: "A group"}
	sub := models.Group{Name: "A sub group"}
	tg := models.Tag{Name: "A tag"}
	var marker models.SceneMarker
	var chapter models.GalleryChapter
	var videoFile models.VideoFile
	var audioFile models.AudioFile
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &editor, ""); err != nil {
			return err
		}
		if err := db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer}); err != nil {
			return err
		}
		if err := db.Scene().Create(ctx, &sc, nil); err != nil {
			return err
		}
		if err := db.Image().Create(ctx, &models.CreateImageInput{Image: &img}); err != nil {
			return err
		}
		if err := db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &gal}); err != nil {
			return err
		}
		if err := db.Gallery().AddImages(ctx, gal.ID, img.ID); err != nil {
			return err
		}
		if err := db.Group().Create(ctx, &grp); err != nil {
			return err
		}
		if err := db.Group().Create(ctx, &sub); err != nil {
			return err
		}
		if err := db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tg}); err != nil {
			return err
		}
		marker = models.SceneMarker{Title: "A marker", SceneID: sc.ID, PrimaryTagID: tg.ID}
		if err := db.SceneMarker().Create(ctx, &marker); err != nil {
			return err
		}
		chapter = models.GalleryChapter{Title: "A chapter", GalleryID: gal.ID, ImageIndex: 1}
		if err := db.GalleryChapter().Create(ctx, &chapter); err != nil {
			return err
		}
		folder := &models.Folder{Path: "/library"}
		if err := db.Folder().Create(ctx, folder); err != nil {
			return err
		}
		videoFile = models.VideoFile{BaseFile: &models.BaseFile{Basename: "a.mp4", ParentFolderID: folder.ID}, Format: "mp4", Width: 1, Height: 1, Duration: 1}
		if err := db.File().Create(ctx, &videoFile); err != nil {
			return err
		}
		audioFile = models.AudioFile{BaseFile: &models.BaseFile{Basename: "a.mp3", ParentFolderID: folder.ID}, Format: "mp3", Duration: 1}
		return db.File().Create(ctx, &audioFile)
	}))

	imageService := &image.Service{File: db.File(), Repository: db.Image()}
	r := &Resolver{
		repository:     db.Repository(),
		hookExecutor:   &mockHookExecutor{},
		groupService:   &group.Service{Repository: db.Group()},
		sceneService:   &scene.Service{File: db.File(), Repository: db.Scene(), MarkerRepository: db.SceneMarker(), Config: config.GetInstance()},
		audioService:   &audio.Service{File: db.File(), Repository: db.Audio()},
		imageService:   imageService,
		galleryService: &gallery.Service{Repository: db.Gallery(), ImageFinder: db.Image(), ImageService: imageService, File: db.File(), Folder: db.Folder()},
	}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	ids := missingParentIDs{
		scene: sc.ID, tag: tg.ID, gallery: gal.ID, group: sub.ID, marker: marker.ID, chapter: chapter.ID,
		videoFile: int(videoFile.ID), audioFile: int(audioFile.ID), performer: performer.ID,
	}
	for mutation, want := range missingParentMutations(ids) {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, editor, "mutation { "+mutation+" }")
			require.Contains(t, body, `"message":"`+want+`"`)
			require.NotContains(t, body, "FOREIGN KEY")
			require.NotContains(t, body, "foreign key")
			require.NotContains(t, body, "does not exist")
			require.NotContains(t, body, "no rows")
		})
	}

	// these succeeded upstream for a missing image or sub group, or a repeated id, and still do
	for mutation, want := range map[string]string{
		fmt.Sprintf(`removeGroupSubGroups(input:{containing_group_id:"%d",sub_group_ids:["999"]})`, grp.ID):    `{"data":{"removeGroupSubGroups":true}}`,
		fmt.Sprintf(`removeGalleryImages(input:{gallery_id:"%d",image_ids:["999"]})`, gal.ID):                  `{"data":{"removeGalleryImages":true}}`,
		fmt.Sprintf(`setGalleryCover(input:{gallery_id:"%d",cover_image_id:"999"})`, gal.ID):                   `{"data":{"setGalleryCover":true}}`,
		fmt.Sprintf(`addGalleryImages(input:{gallery_id:"%d",image_ids:["%d","%d"]})`, gal.ID, img.ID, img.ID): `{"data":{"addGalleryImages":true}}`,
	} {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, editor, "mutation { "+mutation+" }")
			require.JSONEq(t, want, body)
		})
	}

	// and an object that exists is linked as before
	body := storyRequest(t, gql, editor, fmt.Sprintf(`mutation { sceneMarkerCreate(input:{title:"New",seconds:2,scene_id:"%d",primary_tag_id:"%d",tag_ids:["%d","%d"]}) { title } }`, sc.ID, tg.ID, tg.ID, tg.ID))
	require.JSONEq(t, `{"data":{"sceneMarkerCreate":{"title":"New"}}}`, body)
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { galleryChapterCreate(input:{gallery_id:"%d",title:"New",image_index:1}) { title } }`, gal.ID))
	require.JSONEq(t, `{"data":{"galleryChapterCreate":{"title":"New"}}}`, body)
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { addGroupSubGroups(input:{containing_group_id:"%d",sub_groups:[{group_id:"%d"}]}) }`, grp.ID, sub.ID))
	require.JSONEq(t, `{"data":{"addGroupSubGroups":true}}`, body)
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { bulkSceneMarkerUpdate(input:{ids:["%d"],title:"Renamed"}) { title } }`, marker.ID))
	require.JSONEq(t, `{"data":{"bulkSceneMarkerUpdate":[{"title":"Renamed"}]}}`, body)
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { sceneAssignFile(input:{scene_id:"%d",file_id:"%d"}) }`, sc.ID, videoFile.ID))
	require.JSONEq(t, `{"data":{"sceneAssignFile":true}}`, body)
}
