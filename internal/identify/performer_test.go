package identify

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"

	"github.com/stretchr/testify/mock"
)

func Test_getPerformerID(t *testing.T) {
	const (
		emptyEndpoint = ""
		endpoint      = "endpoint"
	)
	invalidStoredID := "invalidStoredID"
	validStoredIDStr := "1"
	validStoredID := 1
	remoteSiteID := "2"
	name := "name"

	db := mocks.NewDatabase()

	db.Performer.On("Create", testCtx, mock.AnythingOfType("*models.CreatePerformerInput")).Run(func(args mock.Arguments) {
		p := args.Get(1).(*models.CreatePerformerInput)
		p.ID = validStoredID
	}).Return(nil)

	type args struct {
		endpoint       string
		p              *models.ScrapedPerformer
		createMissing  bool
		skipSingleName bool
	}
	tests := []struct {
		name    string
		args    args
		want    *int
		wantErr bool
	}{
		{
			"no performer",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{},
				false,
				false,
			},
			nil,
			false,
		},
		{
			"invalid stored id",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					StoredID: &invalidStoredID,
				},
				false,
				false,
			},
			nil,
			true,
		},
		{
			"valid stored id",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					StoredID: &validStoredIDStr,
				},
				false,
				false,
			},
			&validStoredID,
			false,
		},
		{
			"nil stored not creating",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name: &name,
				},
				false,
				false,
			},
			nil,
			false,
		},
		{
			"nil name creating",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{},
				true,
				false,
			},
			nil,
			false,
		},
		{
			"single name no disambig creating",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name: &name,
				},
				true,
				true,
			},
			nil,
			true,
		},
		{
			"valid name creating",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name:         &name,
					RemoteSiteID: &remoteSiteID,
				},
				true,
				false,
			},
			&validStoredID,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPerformerID(testCtx, tt.args.endpoint, db.Performer, tt.args.p, tt.args.createMissing, tt.args.skipSingleName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPerformerID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPerformerID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A new performer gets the first image as the main one and the rest as other
// images, in order.
func Test_createMissingPerformerKeepsAllImages(t *testing.T) {
	name := "Someone"
	main := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnLz4="
	second := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIGlkPSdiJy8+"
	third := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIGlkPSdjJy8+"

	db := mocks.NewDatabase()
	db.Performer.On("Create", testCtx, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.CreatePerformerInput).ID = 7
	}).Return(nil)
	db.Performer.On("UpdateImage", testCtx, 7, []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")).Return(nil).Once()
	var added []string
	db.Performer.On("AddAlternateImage", testCtx, 7, mock.Anything).Run(func(args mock.Arguments) {
		added = append(added, string(args.Get(2).([]byte)))
	}).Return(true, nil)

	got, err := createMissingPerformer(testCtx, "", db.Performer, &models.ScrapedPerformer{
		Name:   &name,
		Images: []string{main, second, "data:image/png;base64,%%%%", third},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *got != 7 {
		t.Errorf("createMissingPerformer() = %d, want 7", *got)
	}
	want := []string{"<svg xmlns='http://www.w3.org/2000/svg' id='b'/>", "<svg xmlns='http://www.w3.org/2000/svg' id='c'/>"}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("other images = %q, want %q", added, want)
	}
	db.AssertExpectations(t)
}

func Test_createMissingPerformer(t *testing.T) {
	emptyEndpoint := ""
	validEndpoint := "validEndpoint"
	remoteSiteID := "remoteSiteID"
	validName := "validName"
	invalidName := "invalidName"
	validImage := "data:image/png;base64,AAAA"
	invalidImage := "data:image/png;base64,%%%%"
	performerID := 1

	db := mocks.NewDatabase()

	db.Performer.On("Create", testCtx, mock.MatchedBy(func(p *models.CreatePerformerInput) bool {
		return p.Name == validName
	})).Run(func(args mock.Arguments) {
		p := args.Get(1).(*models.CreatePerformerInput)
		p.ID = performerID
	}).Return(nil)

	db.Performer.On("Create", testCtx, mock.MatchedBy(func(p *models.CreatePerformerInput) bool {
		return p.Name == invalidName
	})).Return(errors.New("error creating performer"))

	db.Performer.On("UpdateImage", testCtx, performerID, mock.AnythingOfType("[]uint8")).Return(errors.New("error updating performer image"))

	type args struct {
		endpoint string
		p        *models.ScrapedPerformer
	}
	tests := []struct {
		name    string
		args    args
		want    *int
		wantErr bool
	}{
		{
			"simple",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name:         &validName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			&performerID,
			false,
		},
		{
			"error creating",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name:         &invalidName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			nil,
			true,
		},
		{
			"error getting image",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name:         &validName,
					Images:       []string{invalidImage},
					RemoteSiteID: &remoteSiteID,
				},
			},
			nil,
			true,
		},
		{
			"error updating image",
			args{
				emptyEndpoint,
				&models.ScrapedPerformer{
					Name:         &validName,
					Images:       []string{validImage},
					RemoteSiteID: &remoteSiteID,
				},
			},
			nil,
			true,
		},
		{
			"valid stash id",
			args{
				validEndpoint,
				&models.ScrapedPerformer{
					Name:         &validName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			&performerID,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createMissingPerformer(testCtx, tt.args.endpoint, db.Performer, tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("createMissingPerformer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createMissingPerformer() = %v, want %v", got, tt.want)
			}
		})
	}
}
