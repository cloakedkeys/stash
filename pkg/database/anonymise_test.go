//go:build integration
// +build integration

package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/postgres"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func IsPostgresTest() *string {
	if val, ok := os.LookupEnv("PGSQL_TEST"); ok {
		return &val
	}
	return nil
}

const (
	anonUsername    = "anon-secret-username"
	anonNotes       = "anon-secret-notes"
	anonPassword    = "anon-secret-password-hash"
	anonAPIKey      = "anon-secret-api-key-hash"
	anonUIConfig    = "anon-secret-ui-config"
	anonAudioTitle  = "anon-secret-audio-title"
	anonAudioDetail = "anon-secret-audio-details"
	anonAudioCode   = "anon-secret-audio-code"
	anonAudioURL    = "https://anon-secret-audio-url.example.com/"
	anonAudioField  = "anon-secret-audio-field"
	anonVoiceName   = "anon-secret-voice-name"
	anonVoiceDetail = "anon-secret-voice-details"
	anonVoiceAlias  = "anon-secret-voice-alias"
	anonVoiceURL    = "https://anon-secret-voice-url.example.com/"
)

// seedAnonymiserData adds one identifying value to each personal field, and
// returns a function removing them again so that other tests are unaffected.
func seedAnonymiserData(t *testing.T) func() {
	t.Helper()

	var (
		user  models.User
		audio models.Audio
		voice models.VoiceActor
	)

	require.NoError(t, withTxn(func(ctx context.Context) error {
		user = models.User{
			Username: anonUsername,
			Notes:    anonNotes,
			Roles:    models.Roles{models.RoleEnumRead},
		}
		if err := db.User().Create(ctx, &user, anonPassword); err != nil {
			return err
		}
		if err := db.User().SetUserAPIKey(ctx, user.ID, anonAPIKey); err != nil {
			return err
		}

		userCtx := models.WithUserID(ctx, user.ID)
		if err := db.UserUIConfig().Set(userCtx, map[string]interface{}{"name": anonUIConfig}); err != nil {
			return err
		}

		now := time.Now().UTC()
		audio = models.Audio{
			Title:     anonAudioTitle,
			Details:   anonAudioDetail,
			Code:      anonAudioCode,
			URLs:      models.NewRelatedStrings([]string{anonAudioURL}),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := db.Audio().Create(ctx, &audio, nil); err != nil {
			return err
		}
		if err := db.Audio().SetCustomFields(ctx, audio.ID, models.CustomFieldsInput{
			Full: map[string]interface{}{"personal": anonAudioField},
		}); err != nil {
			return err
		}
		if _, err := db.Audio().AddO(userCtx, audio.ID, []time.Time{now}); err != nil {
			return err
		}
		if _, err := db.Audio().AddViews(userCtx, audio.ID, []time.Time{now}); err != nil {
			return err
		}

		voice = models.VoiceActor{
			Name:    anonVoiceName,
			Details: anonVoiceDetail,
			Aliases: []string{anonVoiceAlias},
			URLs:    []string{anonVoiceURL},
		}
		return db.VoiceActor().Create(ctx, &voice)
	}))

	return func() {
		require.NoError(t, withTxn(func(ctx context.Context) error {
			if err := db.VoiceActor().Destroy(ctx, voice.ID); err != nil {
				return err
			}
			if err := db.Audio().Destroy(ctx, audio.ID); err != nil {
				return err
			}
			return db.User().Destroy(ctx, user.ID)
		}))
	}
}

// findInDatabase returns the location of every value in the database file
// containing one of the given strings.
func findInDatabase(t *testing.T, path string, needles []string) []string {
	t.Helper()

	out, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer out.Close()

	rows, err := out.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	require.NoError(t, err)
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Close())
	require.NotEmpty(t, tables)

	var found []string
	for _, table := range tables {
		rows, err := out.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		require.NoError(t, err)

		columns, err := rows.Columns()
		require.NoError(t, err)

		for rows.Next() {
			values := make([]interface{}, len(columns))
			scan := make([]interface{}, len(columns))
			for i := range values {
				scan[i] = &values[i]
			}
			require.NoError(t, rows.Scan(scan...))

			for i, v := range values {
				var text string
				switch x := v.(type) {
				case string:
					text = x
				case []byte:
					text = string(x)
				default:
					continue
				}
				for _, needle := range needles {
					if strings.Contains(text, needle) {
						found = append(found, fmt.Sprintf("%s.%s contains %q", table, columns[i], needle))
					}
				}
			}
		}
		require.NoError(t, rows.Close())
	}

	return found
}

func countRows(t *testing.T, path string, table string) int {
	t.Helper()

	out, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer out.Close()

	var count int
	require.NoError(t, out.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&count))
	return count
}

func TestAnonymiser_Anonymise(t *testing.T) {
	f, err := os.CreateTemp("", "*.sqlite")
	if err != nil {
		t.Errorf("Could not create temporary file: %v", err)
		return
	}

	f.Close()
	defer os.Remove(f.Name())

	defer seedAnonymiserData(t)()

	var wantUsers, wantAudios int
	require.NoError(t, withTxn(func(ctx context.Context) error {
		var err error
		if wantUsers, err = db.User().Count(ctx); err != nil {
			return err
		}
		wantAudios, err = db.Audio().Count(ctx)
		return err
	}))

	// use existing database
	var anonymiser *sqlite.Anonymiser
	if val := IsPostgresTest(); val != nil {
		anonymiser, err = postgres.NewAnonymiser(db.(*postgres.Database), f.Name())
	} else {
		anonymiser, err = sqlite.NewAnonymiser(db.(*sqlite.Database), f.Name())
	}

	if err != nil {
		t.Errorf("Could not create anonymiser: %v", err)
		return
	}

	if err := anonymiser.Anonymise(context.Background()); err != nil {
		t.Errorf("Could not anonymise: %v", err)
		return
	}

	t.Logf("Anonymised database written to %s", f.Name())

	assert.Empty(t, findInDatabase(t, f.Name(), []string{
		anonUsername, anonNotes, anonPassword, anonAPIKey, anonUIConfig,
		anonAudioTitle, anonAudioDetail, anonAudioCode, anonAudioURL, anonAudioField,
		anonVoiceName, anonVoiceDetail, anonVoiceAlias, anonVoiceURL,
	}))

	// the structure must survive, or the result is no use for diagnosing anything
	assert.Equal(t, wantUsers, countRows(t, f.Name(), "users"))
	assert.Equal(t, wantAudios, countRows(t, f.Name(), "audios"))
	for _, table := range []string{"users", "user_roles", "audios", "audio_urls", "audio_custom_fields", "voice_actors", "scenes", "performers", "tags"} {
		assert.NotZero(t, countRows(t, f.Name(), table), "%s should still have rows", table)
	}
	for _, table := range []string{"user_ui_config", "audios_o_dates", "audios_play_dates", "scenes_o_dates", "scenes_view_dates"} {
		assert.Zero(t, countRows(t, f.Name(), table), "%s should be empty", table)
	}
}
