package urlbuilders

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

type AudioURLBuilder struct {
	BaseURL   string
	AudioID   string
	UpdatedAt string
}

func NewAudioURLBuilder(baseURL string, audio *models.Audio) AudioURLBuilder {
	return AudioURLBuilder{
		BaseURL:   baseURL,
		AudioID:   strconv.Itoa(audio.ID),
		UpdatedAt: strconv.FormatInt(audio.UpdatedAt.Unix(), 10),
	}
}

func (b AudioURLBuilder) GetScreenshotURL() string {
	return b.BaseURL + "/audio/" + b.AudioID + "/screenshot?t=" + b.UpdatedAt
}

func (b AudioURLBuilder) GetStreamURL(apiKey string) *url.URL {
	return b.streamURL("stream", apiKey)
}

// GetTranscodeStreamURL returns the url of the live mp3 transcode, used for
// formats that the browser can't play directly.
func (b AudioURLBuilder) GetTranscodeStreamURL(apiKey string) *url.URL {
	return b.streamURL("stream.mp3", apiKey)
}

func (b AudioURLBuilder) streamURL(endpoint string, apiKey string) *url.URL {
	u, err := url.Parse(fmt.Sprintf("%s/audio/%s/%s", b.BaseURL, b.AudioID, endpoint))
	if err != nil {
		// shouldn't happen
		panic(err)
	}

	if apiKey != "" {
		v := u.Query()
		v.Set("apikey", apiKey)
		u.RawQuery = v.Encode()
	}
	return u
}
