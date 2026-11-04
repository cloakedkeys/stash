//go:build integration

package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/audio"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/gallery"
	"github.com/stashapp/stash/pkg/group"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

// An edit that links to a studio, group, tag or file that doesn't exist, or that
// lists one twice, is refused as before, but says which, for example "studio with
// id 999 not found", instead of the database's own error or "tag not found".
// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestEditLinkedObjectErrorsOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)
	cfg := config.GetInstance()

	editor := models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	sc := models.Scene{Title: "A scene"}
	au := models.Audio{Title: "An audio track"}
	img := models.Image{Title: "An image"}
	gal := models.Gallery{Title: "A gallery"}
	g1 := models.Group{Name: "G1"}
	g2 := models.Group{Name: "G2"}
	g3 := models.Group{Name: "G3"}
	st := models.Studio{Name: "A studio"}
	t1 := models.Tag{Name: "T1"}
	t2 := models.Tag{Name: "T2"}
	t3 := models.Tag{Name: "T3"}
	t4 := models.Tag{Name: "T4"}
	var marker models.SceneMarker
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		require.NoError(t, db.User().Create(ctx, &editor, ""))
		require.NoError(t, db.Scene().Create(ctx, &sc, nil))
		require.NoError(t, db.Audio().Create(ctx, &au, nil))
		require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &img}))
		require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &gal}))
		require.NoError(t, db.Group().Create(ctx, &g1))
		require.NoError(t, db.Group().Create(ctx, &g2))
		g3.ContainingGroups = models.NewRelatedGroupDescriptions([]models.GroupIDDescription{{GroupID: g1.ID}})
		require.NoError(t, db.Group().Create(ctx, &g3))
		require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &st}))
		for _, tg := range []*models.Tag{&t1, &t2, &t3, &t4} {
			require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: tg}))
		}
		marker = models.SceneMarker{SceneID: sc.ID, PrimaryTagID: t1.ID, Title: "A marker"}
		return db.SceneMarker().Create(ctx, &marker)
	}))

	repo := db.Repository()
	p := paths.NewPaths(t.TempDir(), "")
	imageService := &image.Service{File: repo.File, Repository: repo.Image}
	r := &Resolver{
		repository:     repo,
		hookExecutor:   &mockHookExecutor{},
		sceneService:   &scene.Service{File: repo.File, Repository: repo.Scene, MarkerRepository: repo.SceneMarker, PluginCache: plugin.NewCache(cfg), Paths: &p, Config: cfg},
		audioService:   &audio.Service{File: repo.File, Repository: repo.Audio, PluginCache: plugin.NewCache(cfg), Paths: &p},
		imageService:   imageService,
		galleryService: &gallery.Service{Repository: repo.Gallery, ImageFinder: repo.Image, ImageService: imageService, File: repo.File, Folder: repo.Folder},
		groupService:   &group.Service{Repository: db.Group()},
	}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	// sceneMarkerUpdate takes its deleter's settings from the manager, which the
	// tests don't start
	previous := newMarkerFileDeleter
	newMarkerFileDeleter = func() *scene.FileDeleter {
		return &scene.FileDeleter{Deleter: file.NewDeleter(), FileNamingAlgo: models.HashAlgorithmOshash, Paths: &p}
	}
	t.Cleanup(func() { newMarkerFileDeleter = previous })

	f := fmt.Sprintf
	refused := []struct{ mutation, want string }{
		// studio_id and parent_id
		{`sceneCreate(input:{title:"x",studio_id:"999"}) { id }`, "studio with id 999 not found"},
		{f(`sceneUpdate(input:{id:"%d",studio_id:"999"}) { id }`, sc.ID), "studio with id 999 not found"},
		{f(`scenesUpdate(input:[{id:"%d",studio_id:"999"}]) { id }`, sc.ID), "studio with id 999 not found"},
		{f(`bulkSceneUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, sc.ID), "studio with id 999 not found"},
		{`audioCreate(input:{title:"x",studio_id:"999"}) { id }`, "studio with id 999 not found"},
		{f(`audioUpdate(input:{id:"%d",studio_id:"999"}) { id }`, au.ID), "studio with id 999 not found"},
		{f(`bulkAudioUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, au.ID), "studio with id 999 not found"},
		{`galleryCreate(input:{title:"x",studio_id:"999"}) { id }`, "studio with id 999 not found"},
		{f(`galleryUpdate(input:{id:"%d",studio_id:"999"}) { id }`, gal.ID), "studio with id 999 not found"},
		{f(`bulkGalleryUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, gal.ID), "studio with id 999 not found"},
		{f(`imageUpdate(input:{id:"%d",studio_id:"999"}) { id }`, img.ID), "studio with id 999 not found"},
		{f(`bulkImageUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, img.ID), "studio with id 999 not found"},
		{`groupCreate(input:{name:"x",studio_id:"999"}) { id }`, "studio with id 999 not found"},
		{f(`groupUpdate(input:{id:"%d",studio_id:"999"}) { id }`, g1.ID), "studio with id 999 not found"},
		{f(`bulkGroupUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, g1.ID), "studio with id 999 not found"},
		{`movieCreate(input:{name:"x",studio_id:"999"}) { id }`, "studio with id 999 not found"},
		{f(`movieUpdate(input:{id:"%d",studio_id:"999"}) { id }`, g1.ID), "studio with id 999 not found"},
		{f(`bulkMovieUpdate(input:{ids:["%d"],studio_id:"999"}) { id }`, g1.ID), "studio with id 999 not found"},
		{`studioCreate(input:{name:"x",parent_id:"999"}) { id }`, "studio with id 999 not found"},

		// a scene's or audio track's groups
		{f(`sceneUpdate(input:{id:"%d",groups:[{group_id:"999"}]}) { id }`, sc.ID), "group with id 999 not found"},
		{`sceneCreate(input:{title:"x",groups:[{group_id:"999"}]}) { id }`, "creating new scene: group with id 999 not found"},
		{f(`bulkSceneUpdate(input:{ids:["%d"],group_ids:{ids:["999"],mode:ADD}}) { id }`, sc.ID), "group with id 999 not found"},
		{f(`audioUpdate(input:{id:"%d",groups:[{group_id:"999"}]}) { id }`, au.ID), "group with id 999 not found"},

		// a group's containing and sub-groups
		{`groupCreate(input:{name:"x",containing_groups:[{group_id:"999"}]}) { id }`, "group with id 999 not found"},
		{`groupCreate(input:{name:"x",sub_groups:[{group_id:"999"}]}) { id }`, "group with id 999 not found"},
		{f(`groupCreate(input:{name:"x",sub_groups:[{group_id:"%d"},{group_id:"%d"}]}) { id }`, g2.ID, g2.ID), f("group with id %d is listed more than once", g2.ID)},
		{f(`groupUpdate(input:{id:"%d",sub_groups:[{group_id:"999"}]}) { id }`, g1.ID), "group with id 999 not found"},
		{f(`groupUpdate(input:{id:"%d",sub_groups:[{group_id:"%d"},{group_id:"%d"}]}) { id }`, g1.ID, g2.ID, g2.ID), f("group with id %d is listed more than once", g2.ID)},
		{f(`bulkGroupUpdate(input:{ids:["%d"],sub_groups:{groups:[{group_id:"999"}],mode:ADD}}) { id }`, g1.ID), "group with id 999 not found"},
		{f(`addGroupSubGroups(input:{containing_group_id:"%d",sub_groups:[{group_id:"%d"},{group_id:"%d"}]})`, g1.ID, g2.ID, g2.ID), f("group with id %d is listed more than once", g2.ID)},

		// join tables: the old error had no id
		{f(`sceneUpdate(input:{id:"%d",tag_ids:["999"]}) { id }`, sc.ID), "tag with id 999 not found"},
		{f(`sceneUpdate(input:{id:"%d",performer_ids:["999"]}) { id }`, sc.ID), "performer with id 999 not found"},
		{f(`sceneUpdate(input:{id:"%d",gallery_ids:["999"]}) { id }`, sc.ID), "gallery with id 999 not found"},
		{`sceneCreate(input:{title:"x",tag_ids:["999"]}) { id }`, "creating new scene: tag with id 999 not found"},
		{f(`audioUpdate(input:{id:"%d",gallery_ids:["999"]}) { id }`, au.ID), "gallery with id 999 not found"},
		{f(`galleryUpdate(input:{id:"%d",scene_ids:["999"]}) { id }`, gal.ID), "scene with id 999 not found"},
		{f(`galleryUpdate(input:{id:"%d",audio_ids:["999"]}) { id }`, gal.ID), "audio with id 999 not found"},
		{f(`groupUpdate(input:{id:"%d",tag_ids:["999"]}) { id }`, g1.ID), "tag with id 999 not found"},
		{f(`studioUpdate(input:{id:"%d",tag_ids:["999"]}) { id }`, st.ID), "tag with id 999 not found"},
		{`tagCreate(input:{name:"x",parent_ids:["999"]}) { id }`, "tag with id 999 not found"},
		{`tagCreate(input:{name:"y",child_ids:["999"]}) { id }`, "tag with id 999 not found"},
		{f(`tagUpdate(input:{id:"%d",parent_ids:["999"]}) { id }`, t1.ID), "tag with id 999 not found"},
		{`performerCreate(input:{name:"x",tag_ids:["999"]}) { id }`, "tag with id 999 not found"},
		{`storyCreate(input:{title:"x",tag_ids:["999"]}) { id }`, "tag with id 999 not found"},
		{f(`bulkSceneMarkerUpdate(input:{ids:["%d"],tag_ids:{ids:["999"],mode:ADD}}) { id }`, marker.ID), "modifying scene marker tags: tag with id 999 not found"},

		// files
		{f(`sceneAssignFile(input:{scene_id:"%d",file_id:"999"})`, sc.ID), "assigning file to scene: file with id 999 not found"},
		{f(`audioAssignFile(input:{audio_id:"%d",file_id:"999"})`, au.ID), "assigning file to audio: file with id 999 not found"},
		{`sceneCreate(input:{title:"x",file_ids:["999"]}) { id }`, "assigning file 999 to new scene: file with id 999 not found"},

		// scene markers
		{f(`sceneMarkerUpdate(input:{id:"%d",scene_id:"999"}) { id }`, marker.ID), "scene with id 999 not found"},
		{f(`sceneMarkerUpdate(input:{id:"%d",primary_tag_id:"999"}) { id }`, marker.ID), "tag with id 999 not found"},
		{f(`sceneMarkerUpdate(input:{id:"%d",tag_ids:["999"]}) { id }`, marker.ID), "tag with id 999 not found"},
		{f(`sceneMarkerUpdate(input:{id:"%d",tag_ids:["%d","%d"]}) { id }`, marker.ID, t2.ID, t2.ID), f("tag with id %d is listed more than once", t2.ID)},
		{f(`sceneMarkerCreate(input:{title:"m",seconds:1,scene_id:"%d",primary_tag_id:"%d",tag_ids:["%d","%d","%d"]}) { id }`, sc.ID, t1.ID, t2.ID, t2.ID, t3.ID), f("tag with id %d is listed more than once", t2.ID)},

		// merging a tag twice
		{f(`tagsMerge(input:{source:["%d","%d"],destination:"%d"}) { id }`, t2.ID, t2.ID, t1.ID), f("tag with id %d is listed more than once", t2.ID)},
	}
	for _, tt := range refused {
		t.Run(tt.mutation, func(t *testing.T) {
			body := storyRequest(t, gql, editor, "mutation { "+tt.mutation+" }")
			require.Contains(t, body, `"message":"`+tt.want+`"`)
			require.NotContains(t, body, "FOREIGN KEY")
			require.NotContains(t, body, "foreign key")
			require.NotContains(t, body, "UNIQUE")
			require.NotContains(t, body, "duplicate key")
			require.NotContains(t, body, "does not exist")
			require.NotContains(t, body, "no rows")
		})
	}

	// what succeeded before still does
	succeeded := []string{
		f(`sceneUpdate(input:{id:"%d",studio_id:"%d"}) { id }`, sc.ID, st.ID),
		f(`sceneUpdate(input:{id:"%d",studio_id:null}) { id }`, sc.ID),
		f(`bulkSceneUpdate(input:{ids:["%d"],studio_id:"%d"}) { id }`, sc.ID, st.ID),
		f(`sceneUpdate(input:{id:"%d",tag_ids:["%d","%d"]}) { id }`, sc.ID, t1.ID, t1.ID),
		f(`bulkSceneUpdate(input:{ids:["%d"],tag_ids:{ids:["999"],mode:REMOVE}}) { id }`, sc.ID),
		f(`bulkSceneUpdate(input:{ids:["%d"],group_ids:{ids:["999"],mode:REMOVE}}) { id }`, sc.ID),
		f(`sceneUpdate(input:{id:"%d",groups:[{group_id:"%d"}]}) { id }`, sc.ID, g1.ID),
		f(`imageUpdate(input:{id:"%d",studio_id:"%d"}) { id }`, img.ID, st.ID),
		f(`groupUpdate(input:{id:"%d",studio_id:"%d"}) { id }`, g2.ID, st.ID),
		f(`bulkGroupUpdate(input:{ids:["%d"],sub_groups:{groups:[{group_id:"999"}],mode:REMOVE}}) { id }`, g1.ID),
		f(`studioCreate(input:{name:"Child",parent_id:"%d"}) { id }`, st.ID),
		f(`tagCreate(input:{name:"Child tag",parent_ids:["%d","%d"]}) { id }`, t1.ID, t1.ID),
		f(`sceneMarkerUpdate(input:{id:"%d",primary_tag_id:"%d",tag_ids:["%d"]}) { id }`, marker.ID, t2.ID, t3.ID),
		f(`addGroupSubGroups(input:{containing_group_id:"%d",sub_groups:[{group_id:"%d"}]})`, g1.ID, g2.ID),
		f(`tagsMerge(input:{source:["%d","%d"],destination:"%d"}) { id }`, t3.ID, t4.ID, t1.ID),
	}
	for _, mutation := range succeeded {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, editor, "mutation { "+mutation+" }")
			require.NotContains(t, body, `"errors"`)
		})
	}
}
