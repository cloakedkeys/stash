package models

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapedPerformerAliases(t *testing.T) {
	name := "Jane"
	legacy := "Alice, Alina"
	for _, tt := range []struct {
		name string
		list []string
		want []string
	}{
		{"legacy", nil, []string{"Alice", "Alina"}},
		{"list takes precedence", []string{"Alice", "Alice, Alina"}, []string{"Alice", "Alice, Alina"}},
		{"empty list takes precedence", []string{}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := ScrapedPerformer{Name: &name, Aliases: &legacy, AliasList: tt.list}
			assert.Equal(t, tt.want, p.ToPerformer("", nil).Aliases.List())
			for _, mode := range []RelationshipUpdateMode{RelationshipUpdateModeSet, RelationshipUpdateModeAdd} {
				partial := p.ToPartial("", nil, map[string]bool{"aliases": mode == RelationshipUpdateModeAdd}, nil)
				require.NotNil(t, partial.Aliases)
				assert.Equal(t, tt.want, partial.Aliases.Values)
				assert.Equal(t, mode, partial.Aliases.Mode)
			}
			excluded := map[string]bool{"aliases": true}
			assert.False(t, p.ToPerformer("", excluded).Aliases.Loaded())
			assert.Nil(t, p.ToPartial("", excluded, nil, nil).Aliases)
		})
	}
}

func Test_scrapedToStudioInput(t *testing.T) {
	const name = "name"
	url := "url"
	url2 := "url2"
	emptyEndpoint := ""
	endpoint := "endpoint"
	remoteSiteID := "remoteSiteID"

	tests := []struct {
		name     string
		studio   *ScrapedStudio
		endpoint string
		want     *Studio
	}{
		{
			"set all",
			&ScrapedStudio{
				Name:         name,
				URLs:         []string{url, url2},
				URL:          &url,
				RemoteSiteID: &remoteSiteID,
			},
			endpoint,
			&Studio{
				Name: name,
				URLs: NewRelatedStrings([]string{url, url2}),
				StashIDs: NewRelatedStashIDs([]StashID{
					{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					},
				}),
			},
		},
		{
			"set url instead of urls",
			&ScrapedStudio{
				Name:         name,
				URL:          &url,
				RemoteSiteID: &remoteSiteID,
			},
			endpoint,
			&Studio{
				Name: name,
				URLs: NewRelatedStrings([]string{url}),
				StashIDs: NewRelatedStashIDs([]StashID{
					{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					},
				}),
			},
		},
		{
			"set none",
			&ScrapedStudio{
				Name: name,
			},
			emptyEndpoint,
			&Studio{
				Name: name,
			},
		},
		{
			"missing remoteSiteID",
			&ScrapedStudio{
				Name: name,
			},
			endpoint,
			&Studio{
				Name: name,
			},
		},
		{
			"set stashid",
			&ScrapedStudio{
				Name:         name,
				RemoteSiteID: &remoteSiteID,
			},
			endpoint,
			&Studio{
				Name: name,
				StashIDs: NewRelatedStashIDs([]StashID{
					{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					},
				}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.studio.ToStudio(tt.endpoint, nil)

			assert.NotEqual(t, time.Time{}, got.CreatedAt)
			assert.NotEqual(t, time.Time{}, got.UpdatedAt)

			got.CreatedAt = time.Time{}
			got.UpdatedAt = time.Time{}
			if got.StashIDs.Loaded() && len(got.StashIDs.List()) > 0 {
				for stid := range got.StashIDs.List() {
					got.StashIDs.List()[stid].UpdatedAt = time.Time{}
				}
			}
			assert.Equal(t, tt.want, got.Studio)
		})
	}
}

func Test_scrapedToPerformerInput(t *testing.T) {
	name := "name"
	emptyEndpoint := ""
	endpoint := "endpoint"
	remoteSiteID := "remoteSiteID"

	const nValues = 19
	stringValues := make([]string, nValues)
	for i := 0; i < nValues; i++ {
		stringValues[i] = strconv.Itoa(i)
	}

	upTo := 0
	nextVal := func() *string {
		ret := stringValues[upTo]
		upTo = (upTo + 1) % len(stringValues)
		return &ret
	}

	nextIntVal := func() *int {
		ret := upTo
		upTo = (upTo + 1) % len(stringValues)
		return &ret
	}

	dateFromInt := func(i int) *Date {
		t := time.Date(2001, 1, i, 0, 0, 0, 0, time.UTC)
		d := Date{Time: t}
		return &d
	}
	dateStrFromInt := func(i int) *string {
		s := dateFromInt(i).String()
		return &s
	}

	genderFromInt := func(i int) *GenderEnum {
		g := AllGenderEnum[i%len(AllGenderEnum)]
		return &g
	}
	genderStrFromInt := func(i int) *string {
		s := genderFromInt(i).String()
		return &s
	}

	tests := []struct {
		name      string
		performer *ScrapedPerformer
		endpoint  string
		want      *Performer
	}{
		{
			"set all",
			&ScrapedPerformer{
				Name:           &name,
				Disambiguation: nextVal(),
				Birthdate:      dateStrFromInt(*nextIntVal()),
				DeathDate:      dateStrFromInt(*nextIntVal()),
				Gender:         genderStrFromInt(*nextIntVal()),
				Ethnicity:      nextVal(),
				Country:        nextVal(),
				EyeColor:       nextVal(),
				HairColor:      nextVal(),
				Height:         nextVal(),
				Weight:         nextVal(),
				Measurements:   nextVal(),
				FakeTits:       nextVal(),
				CareerStart:    dateStrFromInt(2005),
				CareerEnd:      dateStrFromInt(2015),
				Tattoos:        nextVal(),
				Piercings:      nextVal(),
				Aliases:        nextVal(),
				URL:            nextVal(),
				Twitter:        nextVal(),
				Instagram:      nextVal(),
				Details:        nextVal(),
				RemoteSiteID:   &remoteSiteID,
			},
			endpoint,
			&Performer{
				Name:           name,
				Disambiguation: *nextVal(),
				Birthdate:      dateFromInt(*nextIntVal()),
				DeathDate:      dateFromInt(*nextIntVal()),
				Gender:         genderFromInt(*nextIntVal()),
				Ethnicity:      *nextVal(),
				Country:        *nextVal(),
				EyeColor:       *nextVal(),
				HairColor:      *nextVal(),
				Height:         nextIntVal(),
				Weight:         nextIntVal(),
				Measurements:   *nextVal(),
				FakeTits:       *nextVal(),
				CareerStart:    dateFromInt(2005),
				CareerEnd:      dateFromInt(2015),
				Tattoos:        *nextVal(), // skip CareerLength counter slot
				Piercings:      *nextVal(),
				Aliases:        NewRelatedStrings([]string{*nextVal()}),
				URLs:           NewRelatedStrings([]string{*nextVal(), *nextVal(), *nextVal()}),
				Details:        *nextVal(),
				StashIDs: NewRelatedStashIDs([]StashID{
					{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					},
				}),
			},
		},
		{
			"set none",
			&ScrapedPerformer{
				Name: &name,
			},
			emptyEndpoint,
			&Performer{
				Name: name,
			},
		},
		{
			"missing remoteSiteID",
			&ScrapedPerformer{
				Name: &name,
			},
			endpoint,
			&Performer{
				Name: name,
			},
		},
		{
			"set stashid",
			&ScrapedPerformer{
				Name:         &name,
				RemoteSiteID: &remoteSiteID,
			},
			endpoint,
			&Performer{
				Name: name,
				StashIDs: NewRelatedStashIDs([]StashID{
					{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					},
				}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.performer.ToPerformer(tt.endpoint, nil)

			assert.NotEqual(t, time.Time{}, got.CreatedAt)
			assert.NotEqual(t, time.Time{}, got.UpdatedAt)

			got.CreatedAt = time.Time{}
			got.UpdatedAt = time.Time{}

			if got.StashIDs.Loaded() && len(got.StashIDs.List()) > 0 {
				for stid := range got.StashIDs.List() {
					got.StashIDs.List()[stid].UpdatedAt = time.Time{}
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScrapedStudio_ToPartial(t *testing.T) {
	var (
		id                = 1000
		idStr             = strconv.Itoa(id)
		storedID          = "storedID"
		parentStoredID    = 2000
		parentStoredIDStr = strconv.Itoa(parentStoredID)
		name              = "name"
		url               = "url"
		remoteSiteID      = "remoteSiteID"
		endpoint          = "endpoint"
		image             = "image"
		images            = []string{image}

		existingEndpoint = "existingEndpoint"
		existingStashID  = StashID{"existingStashID", existingEndpoint, time.Time{}}
		existingStashIDs = []StashID{existingStashID}
	)

	fullStudio := ScrapedStudio{
		StoredID: &storedID,
		Name:     name,
		URL:      &url,
		Parent: &ScrapedStudio{
			StoredID: &parentStoredIDStr,
		},
		Image:        &image,
		Images:       images,
		RemoteSiteID: &remoteSiteID,
	}

	type args struct {
		id               string
		endpoint         string
		excluded         map[string]bool
		existingStashIDs []StashID
	}

	stdArgs := args{
		id:               idStr,
		endpoint:         endpoint,
		excluded:         map[string]bool{},
		existingStashIDs: existingStashIDs,
	}

	excludeAll := map[string]bool{
		"name":   true,
		"url":    true,
		"parent": true,
	}

	tests := []struct {
		name string
		o    ScrapedStudio
		args args
		want StudioPartial
	}{
		{
			"full no exclusions",
			fullStudio,
			stdArgs,
			StudioPartial{
				ID:   id,
				Name: NewOptionalString(name),
				URLs: &UpdateStrings{
					Values: []string{url},
					Mode:   RelationshipUpdateModeSet,
				},
				ParentID: NewOptionalInt(parentStoredID),
				StashIDs: &UpdateStashIDs{
					StashIDs: append(existingStashIDs, StashID{
						Endpoint: endpoint,
						StashID:  remoteSiteID,
					}),
					Mode: RelationshipUpdateModeSet,
				},
			},
		},
		{
			"exclude all",
			fullStudio,
			args{
				id:       idStr,
				excluded: excludeAll,
			},
			StudioPartial{
				ID: id,
			},
		},
		{
			"overwrite stash id",
			fullStudio,
			args{
				id:               idStr,
				excluded:         excludeAll,
				endpoint:         existingEndpoint,
				existingStashIDs: existingStashIDs,
			},
			StudioPartial{
				ID: id,
				StashIDs: &UpdateStashIDs{
					StashIDs: []StashID{{
						Endpoint: existingEndpoint,
						StashID:  remoteSiteID,
					}},
					Mode: RelationshipUpdateModeSet,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.o
			got := s.ToPartial(tt.args.id, tt.args.endpoint, tt.args.excluded, tt.args.existingStashIDs)

			// unset updatedAt - we don't need to compare it
			got.UpdatedAt = OptionalTime{}
			if got.StashIDs != nil && len(got.StashIDs.StashIDs) > 0 {
				for stid := range got.StashIDs.StashIDs {
					got.StashIDs.StashIDs[stid].UpdatedAt = time.Time{}
				}
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScrapedGetImage(t *testing.T) {
	// Two different 1x1 PNGs keep image selection observable.
	const firstPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="
	const legacyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYPj/HwADAgH/5ncLrgAAAABJRU5ErkJggg=="
	firstBytes, err := base64.StdEncoding.DecodeString(firstPNG)
	require.NoError(t, err)
	legacyBytes, err := base64.StdEncoding.DecodeString(legacyPNG)
	require.NoError(t, err)
	legacy := "data:image/png;base64," + legacyPNG
	images := []string{"data:image/png;base64," + firstPNG, legacy}

	for _, tt := range []struct {
		name     string
		image    *string
		images   []string
		excluded map[string]bool
		want     []byte
	}{
		{name: "only Images", images: images, want: firstBytes},
		{name: "only Image", image: &legacy, want: legacyBytes},
		{name: "both", image: &legacy, images: images, want: firstBytes},
		{name: "neither"},
		{name: "empty Images", image: &legacy, images: []string{}, want: legacyBytes},
		{name: "excluded Images", images: images, excluded: map[string]bool{"image": true}},
		{name: "excluded Image", image: &legacy, excluded: map[string]bool{"image": true}},
		{name: "excluded both", image: &legacy, images: images, excluded: map[string]bool{"image": true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			studio := ScrapedStudio{Image: tt.image, Images: tt.images}
			performer := ScrapedPerformer{Image: tt.image, Images: tt.images}
			for _, subject := range []struct {
				name     string
				getImage func(context.Context, map[string]bool) ([]byte, error)
			}{
				{name: "studio", getImage: studio.GetImage},
				{name: "performer", getImage: performer.GetImage},
			} {
				t.Run(subject.name, func(t *testing.T) {
					got, err := subject.getImage(context.Background(), tt.excluded)
					require.NoError(t, err)
					assert.Equal(t, tt.want, got)
				})
			}
		})
	}
}
