//go:build integration

package database_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/group"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// threeGroups are new groups, unlinked, for one test case.
type threeGroups struct{ a, b, c *models.Group }

func newThreeGroups(ctx context.Context, t *testing.T) threeGroups {
	t.Helper()
	var ret [3]*models.Group
	for i, name := range []string{"relation store A", "relation store B", "relation store C"} {
		g := models.NewGroup()
		g.Name = name
		require.NoError(t, db.Group().Create(ctx, &g))
		ret[i] = &g
	}
	return threeGroups{ret[0], ret[1], ret[2]}
}

func groupDescriptions(ids ...int) []models.GroupIDDescription {
	ret := make([]models.GroupIDDescription, len(ids))
	for i, id := range ids {
		ret[i] = models.GroupIDDescription{GroupID: id}
	}
	return ret
}

func updateGroupLinks(ctx context.Context, id int, containing bool, mode models.RelationshipUpdateMode, ids ...int) error {
	p := models.NewGroupPartial()
	d := &models.UpdateGroupDescriptions{Groups: groupDescriptions(ids...), Mode: mode}
	if containing {
		p.ContainingGroups = d
	} else {
		p.SubGroups = d
	}
	_, err := db.Group().UpdatePartial(ctx, id, p)
	return err
}

func createGroupWithLinks(ctx context.Context, containing bool, ids ...int) error {
	g := models.NewGroup()
	g.Name = "relation store new"
	if containing {
		g.ContainingGroups = models.NewRelatedGroupDescriptions(groupDescriptions(ids...))
	} else {
		g.SubGroups = models.NewRelatedGroupDescriptions(groupDescriptions(ids...))
	}
	return db.Group().Create(ctx, &g)
}

// runGroupRelationCase runs f on new groups in a transaction that is rolled
// back, and returns its error. On PostgreSQL a failed statement ends the
// transaction, so each case has its own.
func runGroupRelationCase(t *testing.T, f func(ctx context.Context, g threeGroups) error) error {
	t.Helper()
	var err error
	runRollbackTxn(t, func(ctx context.Context) error {
		err = f(ctx, newThreeGroups(ctx, t))
		return nil
	})
	return err
}

// TestGroupRelationStoreErrors calls the group, scene and audio stores
// directly, without the service's checks, as a group import does. A group given
// twice, or added when it is already linked, says which; any other refused row
// keeps the database's own error, naming the table once (#2085).
func TestGroupRelationStoreErrors(t *testing.T) {
	const (
		containing = true
		sub        = false
		set        = models.RelationshipUpdateModeSet
		add        = models.RelationshipUpdateModeAdd
	)
	listed := func(id int) string { return fmt.Sprintf("group with id %d is listed more than once", id) }

	tests := []struct {
		name string
		run  func(ctx context.Context, g threeGroups) error
		want func(g threeGroups) string
	}{
		{
			"create with a sub-group twice",
			func(ctx context.Context, g threeGroups) error { return createGroupWithLinks(ctx, sub, g.b.ID, g.b.ID) },
			func(g threeGroups) string { return listed(g.b.ID) },
		},
		{
			"create with a containing group twice",
			func(ctx context.Context, g threeGroups) error {
				return createGroupWithLinks(ctx, containing, g.a.ID, g.a.ID)
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"set a sub-group twice",
			func(ctx context.Context, g threeGroups) error {
				return updateGroupLinks(ctx, g.a.ID, sub, set, g.b.ID, g.b.ID)
			},
			func(g threeGroups) string { return listed(g.b.ID) },
		},
		{
			"set a containing group twice",
			func(ctx context.Context, g threeGroups) error {
				return updateGroupLinks(ctx, g.b.ID, containing, set, g.a.ID, g.a.ID)
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"add a sub-group twice",
			func(ctx context.Context, g threeGroups) error {
				return updateGroupLinks(ctx, g.a.ID, sub, add, g.c.ID, g.b.ID, g.b.ID)
			},
			func(g threeGroups) string { return listed(g.b.ID) },
		},
		{
			"add a containing group twice",
			func(ctx context.Context, g threeGroups) error {
				return updateGroupLinks(ctx, g.c.ID, containing, add, g.a.ID, g.a.ID)
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"add a sub-group that is already linked",
			func(ctx context.Context, g threeGroups) error {
				if err := updateGroupLinks(ctx, g.a.ID, sub, add, g.b.ID); err != nil {
					return err
				}
				return updateGroupLinks(ctx, g.a.ID, sub, add, g.c.ID, g.b.ID)
			},
			func(g threeGroups) string {
				return fmt.Sprintf("group with id %d is already a sub-group of group with id %d", g.b.ID, g.a.ID)
			},
		},
		{
			"add a containing group that is already linked",
			func(ctx context.Context, g threeGroups) error {
				if err := updateGroupLinks(ctx, g.a.ID, sub, add, g.b.ID); err != nil {
					return err
				}
				return updateGroupLinks(ctx, g.b.ID, containing, add, g.a.ID)
			},
			func(g threeGroups) string {
				return fmt.Sprintf("group with id %d is already a containing group of group with id %d", g.a.ID, g.b.ID)
			},
		},
		{
			"import with a sub-group twice",
			func(ctx context.Context, g threeGroups) error {
				i := group.Importer{
					ReaderWriter: db.Group(),
					StudioWriter: db.Studio(),
					TagWriter:    db.Tag(),
					Input: jsonschema.Group{
						Name:      "relation store imported",
						SubGroups: []jsonschema.SubGroupDescription{{Group: g.b.Name}, {Group: g.b.Name}},
					},
					MissingRefBehaviour: models.ImportMissingRefEnumFail,
				}
				if err := i.PreImport(ctx); err != nil {
					return err
				}
				id, err := i.Create(ctx)
				if err != nil {
					return err
				}
				return i.PostImport(ctx, *id)
			},
			func(g threeGroups) string { return "error setting parents: " + listed(g.b.ID) },
		},
		{
			"create a scene with a group twice",
			func(ctx context.Context, g threeGroups) error {
				s := models.Scene{Title: "relation store", Groups: models.NewRelatedGroups([]models.GroupsScenes{{GroupID: g.a.ID}, {GroupID: g.a.ID}})}
				return db.Scene().Create(ctx, &s, nil)
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"set a scene's group twice",
			func(ctx context.Context, g threeGroups) error {
				p := models.NewScenePartial()
				p.GroupIDs = &models.UpdateGroupIDs{Groups: []models.GroupsScenes{{GroupID: g.a.ID}, {GroupID: g.a.ID}}, Mode: set}
				_, err := db.Scene().UpdatePartial(ctx, sceneIDs[0], p)
				return err
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"create an audio track with a group twice",
			func(ctx context.Context, g threeGroups) error {
				a := models.Audio{Title: "relation store", Groups: models.NewRelatedGroupsAudio([]models.GroupsAudios{{GroupID: g.a.ID}, {GroupID: g.a.ID}})}
				return db.Audio().Create(ctx, &a, nil)
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
		{
			"set an audio track's group twice",
			func(ctx context.Context, g threeGroups) error {
				a := models.Audio{Title: "relation store"}
				if err := db.Audio().Create(ctx, &a, nil); err != nil {
					return err
				}
				p := models.NewAudioPartial()
				p.GroupIDs = &models.UpdateGroupIDsAudio{Groups: []models.GroupsAudios{{GroupID: g.a.ID}, {GroupID: g.a.ID}}, Mode: set}
				_, err := db.Audio().UpdatePartial(ctx, a.ID, p)
				return err
			},
			func(g threeGroups) string { return listed(g.a.ID) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var want string
			err := runGroupRelationCase(t, func(ctx context.Context, g threeGroups) error {
				want = tt.want(g)
				return tt.run(ctx, g)
			})
			require.Error(t, err)
			// a scene or audio create adds what it was doing
			assert.True(t, strings.HasSuffix(err.Error(), want), "got %q, want it to end %q", err.Error(), want)
			assert.NotContains(t, err.Error(), "inserting into")
		})
	}
}

// orderIndexClash adds a trigger that, before any group but other is linked under
// a containing group, links other there with the same order_index. That row then breaks
// the containing group's order_index UNIQUE rule, not its primary key, as two
// edits at once can on PostgreSQL. It lasts until the transaction ends.
func orderIndexClash(ctx context.Context, t *testing.T, other int) {
	t.Helper()
	stmts := []string{fmt.Sprintf(`CREATE TRIGGER relation_store_clash BEFORE INSERT ON groups_relations WHEN NEW.sub_id <> %d
BEGIN INSERT INTO groups_relations (containing_id, sub_id, order_index) VALUES (NEW.containing_id, %d, NEW.order_index); END`, other, other)}
	if IsPostgresTest() != nil {
		stmts = []string{
			fmt.Sprintf(`CREATE FUNCTION relation_store_clash() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.sub_id <> %d THEN
    INSERT INTO groups_relations (containing_id, sub_id, order_index) VALUES (NEW.containing_id, %d, NEW.order_index);
  END IF;
  RETURN NEW;
END $$`, other, other),
			`CREATE TRIGGER relation_store_clash BEFORE INSERT ON groups_relations FOR EACH ROW EXECUTE FUNCTION relation_store_clash()`,
		}
	}
	for _, s := range stmts {
		_, _, err := db.ExecSQL(ctx, s, nil)
		require.NoError(t, err)
	}
}

// TestGroupRelationOrderIndexClash checks that a containing or sub-group row
// refused by the order_index UNIQUE rule keeps the database's own error, named
// once, and isn't reported as a repeat or an existing link (#2085).
func TestGroupRelationOrderIndexClash(t *testing.T) {
	const (
		containing = true
		sub        = false
	)
	tests := []struct {
		name string
		run  func(ctx context.Context, g threeGroups) error
	}{
		{"create with a sub-group", func(ctx context.Context, g threeGroups) error {
			return createGroupWithLinks(ctx, sub, g.b.ID)
		}},
		{"create with a containing group", func(ctx context.Context, g threeGroups) error {
			return createGroupWithLinks(ctx, containing, g.a.ID)
		}},
		{"set a sub-group", func(ctx context.Context, g threeGroups) error {
			return updateGroupLinks(ctx, g.a.ID, sub, models.RelationshipUpdateModeSet, g.b.ID)
		}},
		{"add a sub-group", func(ctx context.Context, g threeGroups) error {
			return updateGroupLinks(ctx, g.a.ID, sub, models.RelationshipUpdateModeAdd, g.b.ID)
		}},
		{"add a containing group", func(ctx context.Context, g threeGroups) error {
			return updateGroupLinks(ctx, g.b.ID, containing, models.RelationshipUpdateModeAdd, g.a.ID)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runGroupRelationCase(t, func(ctx context.Context, g threeGroups) error {
				orderIndexClash(ctx, t, g.c.ID)
				return tt.run(ctx, g)
			})
			require.Error(t, err)
			msg := err.Error()
			assert.Contains(t, msg, "order_index")
			assert.Equal(t, 1, strings.Count(msg, "inserting into groups_relations"), msg)
			assert.NotContains(t, msg, "already a")
			assert.NotContains(t, msg, "listed more than once")
		})
	}
}
