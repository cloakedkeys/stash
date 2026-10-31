package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/models"
)

type VoiceActorStore struct{}

type voiceActorRow struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Details   string    `db:"details"`
	Aliases   string    `db:"aliases"`
	URLs      string    `db:"urls"`
	Favorite  bool      `db:"favorite"`
	CreatedAt Timestamp `db:"created_at"`
	UpdatedAt Timestamp `db:"updated_at"`
}

func (r voiceActorRow) resolve() (*models.VoiceActor, error) {
	v := &models.VoiceActor{ID: r.ID, Name: r.Name, Details: r.Details, Favorite: r.Favorite, CreatedAt: r.CreatedAt.Timestamp, UpdatedAt: r.UpdatedAt.Timestamp, Aliases: []string{}, URLs: []string{}}
	if err := json.Unmarshal([]byte(r.Aliases), &v.Aliases); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(r.URLs), &v.URLs); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *VoiceActorStore) find(ctx context.Context, where string, args ...interface{}) (*models.VoiceActor, error) {
	var row voiceActorRow
	err := dbWrapper.Get(ctx, &row, "SELECT * FROM voice_actors WHERE "+where, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.resolve()
}

func (s *VoiceActorStore) Find(ctx context.Context, id int) (*models.VoiceActor, error) {
	return s.find(ctx, "id = ?", id)
}
func (s *VoiceActorStore) FindByName(ctx context.Context, name string) (*models.VoiceActor, error) {
	return s.find(ctx, "name = ? COLLATE NOCASE", strings.TrimSpace(name))
}

func (s *VoiceActorStore) Query(ctx context.Context, filter *models.FindFilterType, favorite *bool) ([]*models.VoiceActor, int, error) {
	where := "1 = 1"
	args := []interface{}{}
	page, perPage := 1, 40
	order := "name COLLATE NATURAL_CI ASC, id ASC"
	if filter != nil {
		if filter.Q != nil && *filter.Q != "" {
			where += " AND (name LIKE ? OR EXISTS (SELECT 1 FROM json_each(voice_actors.aliases) WHERE value LIKE ?))"
			q := "%" + *filter.Q + "%"
			args = append(args, q, q)
		}
		if filter.Page != nil && *filter.Page > 0 {
			page = *filter.Page
		}
		if filter.PerPage != nil {
			perPage = *filter.PerPage
		}
		if filter.Direction != nil && *filter.Direction == models.SortDirectionEnumDesc {
			order = "name COLLATE NATURAL_CI DESC, id DESC"
		}
	}
	if favorite != nil {
		where += " AND favorite = ?"
		args = append(args, *favorite)
	}
	var count int
	if err := dbWrapper.Get(ctx, &count, "SELECT COUNT(*) FROM voice_actors WHERE "+where, args...); err != nil {
		return nil, 0, err
	}
	query := "SELECT * FROM voice_actors WHERE " + where + " ORDER BY " + order
	if perPage != -1 {
		if perPage < 1 {
			perPage = 40
		}
		query += " LIMIT ? OFFSET ?"
		args = append(args, perPage, (page-1)*perPage)
	}
	var rows []voiceActorRow
	if err := dbWrapper.Select(ctx, &rows, query, args...); err != nil {
		return nil, 0, err
	}
	ret := make([]*models.VoiceActor, 0, len(rows))
	for _, row := range rows {
		v, err := row.resolve()
		if err != nil {
			return nil, 0, err
		}
		ret = append(ret, v)
	}
	return ret, count, nil
}

func voiceActorValues(v *models.VoiceActor) ([]interface{}, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return nil, fmt.Errorf("voice actor name is required")
	}
	clean := func(values []string, foldCase bool) []string {
		ret := []string{}
		seen := map[string]bool{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			key := value
			if foldCase {
				key = strings.ToLower(value)
			}
			if value != "" && !seen[key] {
				ret = append(ret, value)
				seen[key] = true
			}
		}
		return ret
	}
	v.Aliases = clean(v.Aliases, true)
	v.URLs = clean(v.URLs, false)
	aliases, err := json.Marshal(v.Aliases)
	if err != nil {
		return nil, err
	}
	urls, err := json.Marshal(v.URLs)
	if err != nil {
		return nil, err
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	return []interface{}{v.Name, v.Details, string(aliases), string(urls), v.Favorite, Timestamp{Timestamp: v.CreatedAt}, Timestamp{Timestamp: v.UpdatedAt}}, nil
}

func (s *VoiceActorStore) Create(ctx context.Context, v *models.VoiceActor) error {
	args, err := voiceActorValues(v)
	if err != nil {
		return err
	}
	result, err := dbWrapper.Exec(ctx, "INSERT INTO voice_actors (name,details,aliases,urls,favorite,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", args...)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	v.ID = int(id)
	return err
}
func (s *VoiceActorStore) Update(ctx context.Context, v *models.VoiceActor) error {
	args, err := voiceActorValues(v)
	if err != nil {
		return err
	}
	args = append(args, v.ID)
	_, err = dbWrapper.Exec(ctx, "UPDATE voice_actors SET name=?,details=?,aliases=?,urls=?,favorite=?,created_at=?,updated_at=? WHERE id=?", args...)
	return err
}
func (s *VoiceActorStore) Destroy(ctx context.Context, id int) error {
	_, err := dbWrapper.Exec(ctx, "DELETE FROM voice_actors WHERE id=?", id)
	return err
}
func (s *VoiceActorStore) FindBySceneID(ctx context.Context, id int) ([]*models.VoiceActor, error) {
	var rows []voiceActorRow
	if err := dbWrapper.Select(ctx, &rows, "SELECT v.* FROM voice_actors v JOIN scenes_voice_actors c ON c.voice_actor_id=v.id WHERE c.scene_id=? ORDER BY v.name COLLATE NATURAL_CI,v.id", id); err != nil {
		return nil, err
	}
	ret := make([]*models.VoiceActor, 0, len(rows))
	for _, row := range rows {
		v, err := row.resolve()
		if err != nil {
			return nil, err
		}
		ret = append(ret, v)
	}
	return ret, nil
}
func (s *VoiceActorStore) SceneIDs(ctx context.Context, id int) ([]int, error) {
	ret := []int{}
	err := dbWrapper.Select(ctx, &ret, "SELECT scene_id FROM scenes_voice_actors WHERE voice_actor_id=? ORDER BY scene_id", id)
	return ret, err
}
func (s *VoiceActorStore) SetScene(ctx context.Context, id int, ids []int) error {
	if _, err := dbWrapper.Exec(ctx, "DELETE FROM scenes_voice_actors WHERE scene_id=?", id); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, actorID := range ids {
		if seen[actorID] {
			continue
		}
		seen[actorID] = true
		if _, err := dbWrapper.Exec(ctx, "INSERT INTO scenes_voice_actors (scene_id,voice_actor_id) VALUES (?,?)", id, actorID); err != nil {
			return err
		}
	}
	return nil
}

// MergeScenes preserves voice credits before the source scenes are destroyed.
func (s *VoiceActorStore) MergeScenes(ctx context.Context, destination int, sources []int) error {
	for _, source := range sources {
		if _, err := dbWrapper.Exec(ctx, "INSERT OR IGNORE INTO scenes_voice_actors (scene_id, voice_actor_id) SELECT ?, voice_actor_id FROM scenes_voice_actors WHERE scene_id = ?", destination, source); err != nil {
			return err
		}
	}
	return nil
}
