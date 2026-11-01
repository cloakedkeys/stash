package urlbuilders

import (
	"net/url"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

type PerformerURLBuilder struct {
	BaseURL     string
	PerformerID string
	UpdatedAt   string
}

func NewPerformerURLBuilder(baseURL string, performer *models.Performer) PerformerURLBuilder {
	return PerformerURLBuilder{
		BaseURL:     baseURL,
		PerformerID: strconv.Itoa(performer.ID),
		UpdatedAt:   strconv.FormatInt(performer.UpdatedAt.Unix(), 10),
	}
}

func (b PerformerURLBuilder) GetPerformerImageURL(hasImage bool) string {
	url := b.BaseURL + "/performer/" + b.PerformerID + "/image?t=" + b.UpdatedAt
	if !hasImage {
		url += "&default=true"
	}
	return url
}

func (b PerformerURLBuilder) GetAlternateImageURL(checksum string) string {
	query := url.Values{"t": []string{checksum}}
	return b.BaseURL + "/performer/" + b.PerformerID + "/image/" + url.PathEscape(checksum) + "?" + query.Encode()
}
