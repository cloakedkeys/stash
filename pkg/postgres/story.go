package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"
	"gopkg.in/guregu/null.v4"
	"gopkg.in/guregu/null.v4/zero"

	"github.com/stashapp/stash/pkg/librarylimit"
	"github.com/stashapp/stash/pkg/models"
)

const (
	storyTable           = "stories"
	storiesFilesTable    = "stories_files"
	storyIDColumn        = "story_id"
	storyURLColumn       = "url"
	storiesTagsTable     = "stories_tags"
	storiesURLsTable     = "story_urls"
	storiesUserDataTable = "stories_user_data"
)

var storyUserData = userDataTable{
	table:       storiesUserDataTable,
	idColumn:    storyIDColumn,
	objectTable: storyTable,
}

type storyRow struct {
	ID            int         `db:"id" goqu:"skipinsert"`
	Title         zero.String `db:"title"`
	Details       zero.String `db:"details"`
	Author        zero.String `db:"author"`
	SourceWebsite zero.String `db:"source_website"`
	Language      zero.String `db:"language"`
	Tagline       zero.String `db:"tagline"`
	Date          NullDate    `db:"date"`
	DatePrecision null.Int    `db:"date_precision"`
	CreatedAt     Timestamp   `db:"created_at"`
	UpdatedAt     Timestamp   `db:"updated_at"`
}

func (r *storyRow) fromStory(o models.Story) {
	r.ID = o.ID
	r.Title = zero.StringFrom(o.Title)
	r.Details = zero.StringFrom(o.Details)
	r.Author = zero.StringFrom(o.Author)
	r.SourceWebsite = zero.StringFrom(o.SourceWebsite)
	r.Language = zero.StringFrom(o.Language)
	r.Tagline = zero.StringFrom(o.Tagline)
	r.Date = NullDateFromDatePtr(o.Date)
	r.DatePrecision = datePrecisionFromDatePtr(o.Date)
	r.CreatedAt = Timestamp{Timestamp: o.CreatedAt}
	r.UpdatedAt = Timestamp{Timestamp: o.UpdatedAt}
}

type storyUserDataRow struct {
	StoryID int      `db:"story_id"`
	UserID  int      `db:"user_id"`
	Rating  null.Int `db:"rating"`
}

func (r storyUserDataRow) apply(a *models.Story) {
	a.Rating = nullIntPtr(r.Rating)
}

func storyUserDataRecord(a models.Story) goqu.Record {
	return goqu.Record{
		"rating": intFromPtr(a.Rating),
	}
}

func storyUserDataPartialRecord(a models.StoryPartial) goqu.Record {
	r := updateRecord{Record: make(exp.Record)}
	r.setNullInt("rating", a.Rating)
	return goqu.Record(r.Record)
}

type storyQueryRow struct {
	storyRow
	PrimaryFileID         null.Int    `db:"primary_file_id"`
	PrimaryFileFolderPath zero.String `db:"primary_file_folder_path"`
	PrimaryFileBasename   zero.String `db:"primary_file_basename"`
}

func (r *storyQueryRow) resolve() *models.Story {
	ret := &models.Story{
		ID:            r.ID,
		Title:         r.Title.String,
		Details:       r.Details.String,
		Author:        r.Author.String,
		SourceWebsite: r.SourceWebsite.String,
		Language:      r.Language.String,
		Tagline:       r.Tagline.String,
		Date:          r.Date.DatePtr(r.DatePrecision),

		PrimaryFileID: nullIntFileIDPtr(r.PrimaryFileID),

		CreatedAt: r.CreatedAt.Timestamp.UTC(),
		UpdatedAt: r.UpdatedAt.Timestamp.UTC(),
	}

	if r.PrimaryFileFolderPath.Valid && r.PrimaryFileBasename.Valid {
		ret.Path = filepath.Join(r.PrimaryFileFolderPath.String, r.PrimaryFileBasename.String)
	}

	return ret
}

type storyRowRecord struct {
	updateRecord
}

func (r *storyRowRecord) fromPartial(o models.StoryPartial) {
	r.setNullString("title", o.Title)
	r.setNullString("details", o.Details)
	r.setNullString("author", o.Author)
	r.setNullString("source_website", o.SourceWebsite)
	r.setNullString("language", o.Language)
	r.setNullString("tagline", o.Tagline)
	r.setNullDate("date", "date_precision", o.Date)
	r.setTimestamp("created_at", o.CreatedAt)
	r.setTimestamp("updated_at", o.UpdatedAt)
}

type storyRepositoryType struct {
	repository
	tags  joinRepository
	files filesRepository
}

var storyRepository = storyRepositoryType{
	repository: repository{tableName: storyTable, idColumn: idColumn},
	tags:       joinRepository{repository: repository{tableName: storiesTagsTable, idColumn: storyIDColumn}, fkColumn: tagIDColumn, foreignTable: tagTable, orderBy: tagTableSortSQL},
	files:      filesRepository{repository: repository{tableName: storiesFilesTable, idColumn: storyIDColumn}},
}

type StoryStore struct {
	tableMgr *table
	repo     *storeRepository
}

func NewStoryStore(r *storeRepository) *StoryStore {
	return &StoryStore{tableMgr: storyTableMgr, repo: r}
}

func (qb *StoryStore) table() exp.IdentifierExpression {
	return qb.tableMgr.table
}

func (qb *StoryStore) selectDataset() *goqu.SelectDataset {
	table := qb.table()
	files := fileTableMgr.table
	folders := folderTableMgr.table

	return dialect.From(table).LeftJoin(
		storiesFilesJoinTable,
		goqu.On(
			storiesFilesJoinTable.Col(storyIDColumn).Eq(table.Col(idColumn)),
			storiesFilesJoinTable.Col("primary").IsTrue(),
		),
	).LeftJoin(
		files,
		goqu.On(files.Col(idColumn).Eq(storiesFilesJoinTable.Col(fileIDColumn))),
	).LeftJoin(
		folders,
		goqu.On(folders.Col(idColumn).Eq(files.Col("parent_folder_id"))),
	).Select(
		qb.table().All(),
		storiesFilesJoinTable.Col(fileIDColumn).As("primary_file_id"),
		folders.Col("path").As("primary_file_folder_path"),
		files.Col("basename").As("primary_file_basename"),
	)
}

func (qb *StoryStore) Create(ctx context.Context, newObject *models.Story, fileIDs []models.FileID) error {
	var r storyRow
	r.fromStory(*newObject)

	id, err := qb.tableMgr.insertID(ctx, r)
	if err != nil {
		return err
	}
	if err := storyUserData.upsert(ctx, id, storyUserDataRecord(*newObject)); err != nil {
		return err
	}

	if len(fileIDs) > 0 {
		const firstPrimary = true
		if err := storiesFilesTableMgr.insertJoins(ctx, id, firstPrimary, fileIDs); err != nil {
			return err
		}
	}

	if newObject.URLs.Loaded() {
		const startPos = 0
		if err := storiesURLsTableMgr.insertJoins(ctx, id, startPos, newObject.URLs.List()); err != nil {
			return err
		}
	}

	if newObject.TagIDs.Loaded() {
		if err := storiesTagsTableMgr.insertJoins(ctx, id, newObject.TagIDs.List()); err != nil {
			return err
		}
	}

	updated, err := qb.find(ctx, id)
	if err != nil {
		return fmt.Errorf("finding after create: %w", err)
	}

	*newObject = *updated

	return nil
}

func (qb *StoryStore) UpdatePartial(ctx context.Context, id int, partial models.StoryPartial) (*models.Story, error) {
	r := storyRowRecord{
		updateRecord{
			Record: make(exp.Record),
		},
	}

	r.fromPartial(partial)

	if len(r.Record) > 0 {
		if err := qb.tableMgr.updateByID(ctx, id, r.Record); err != nil {
			return nil, err
		}
	}
	if err := storyUserData.upsert(ctx, id, storyUserDataPartialRecord(partial)); err != nil {
		return nil, err
	}

	if partial.URLs != nil {
		if err := storiesURLsTableMgr.modifyJoins(ctx, id, partial.URLs.Values, partial.URLs.Mode); err != nil {
			return nil, err
		}
	}

	if partial.TagIDs != nil {
		if err := storiesTagsTableMgr.modifyJoins(ctx, id, partial.TagIDs.IDs, partial.TagIDs.Mode); err != nil {
			return nil, err
		}
	}

	if partial.PrimaryFileID != nil {
		if err := storiesFilesTableMgr.setPrimary(ctx, id, *partial.PrimaryFileID); err != nil {
			return nil, err
		}
	}

	return qb.find(ctx, id)
}

func (qb *StoryStore) Update(ctx context.Context, updatedObject *models.Story) error {
	var r storyRow
	r.fromStory(*updatedObject)

	if err := qb.tableMgr.updateByID(ctx, updatedObject.ID, r); err != nil {
		return err
	}
	if err := storyUserData.upsert(ctx, updatedObject.ID, storyUserDataRecord(*updatedObject)); err != nil {
		return err
	}

	if updatedObject.URLs.Loaded() {
		if err := storiesURLsTableMgr.replaceJoins(ctx, updatedObject.ID, updatedObject.URLs.List()); err != nil {
			return err
		}
	}

	if updatedObject.TagIDs.Loaded() {
		if err := storiesTagsTableMgr.replaceJoins(ctx, updatedObject.ID, updatedObject.TagIDs.List()); err != nil {
			return err
		}
	}

	if updatedObject.Files.Loaded() {
		fileIDs := make([]models.FileID, len(updatedObject.Files.List()))
		for i, f := range updatedObject.Files.List() {
			fileIDs[i] = f.Base().ID
		}
		if err := storiesFilesTableMgr.replaceJoins(ctx, updatedObject.ID, fileIDs); err != nil {
			return err
		}
	}
	return nil
}

func (qb *StoryStore) Destroy(ctx context.Context, id int) error {
	// Cascading foreign keys remove joins and per-user data only.
	// File records and source files are retained.

	return qb.tableMgr.destroyExisting(ctx, []int{id})
}

func (qb *StoryStore) Find(ctx context.Context, id int) (*models.Story, error) {
	ret, err := qb.find(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ret, err
}

func (qb *StoryStore) FindByIDs(ctx context.Context, ids []int) ([]*models.Story, error) {
	stories := make([]*models.Story, 0, len(ids))

	table := qb.table()
	if err := batchExec(ids, defaultBatchSize, func(batch []int) error {
		q := qb.selectDataset().Prepared(true).Where(table.Col(idColumn).In(batch))
		unsorted, err := qb.getMany(ctx, q)
		if err != nil {
			return err
		}

		stories = append(stories, unsorted...)

		return nil
	}); err != nil {
		return nil, err
	}

	return stories, nil
}

func (qb *StoryStore) FindMany(ctx context.Context, ids []int) ([]*models.Story, error) {
	ids, limitErr := visibleIDs(ctx, librarylimit.Stories, ids)
	if limitErr != nil {
		return nil, limitErr
	}

	stories := make([]*models.Story, len(ids))

	unsorted, err := qb.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, s := range unsorted {
		i := slices.Index(ids, s.ID)
		stories[i] = s
	}

	for i := range stories {
		if stories[i] == nil {
			return nil, fmt.Errorf("story with id %d not found", ids[i])
		}
	}

	return stories, nil
}

func (qb *StoryStore) find(ctx context.Context, id int) (*models.Story, error) {
	q := qb.selectDataset().Where(qb.tableMgr.byID(id))

	ret, err := qb.get(ctx, q)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (qb *StoryStore) findBySubquery(ctx context.Context, sq *goqu.SelectDataset) ([]*models.Story, error) {
	table := qb.table()

	q := qb.selectDataset().Where(
		table.Col(idColumn).In(
			sq,
		),
	)

	return qb.getMany(ctx, q)
}

func (qb *StoryStore) get(ctx context.Context, q *goqu.SelectDataset) (*models.Story, error) {
	ret, err := qb.getMany(ctx, q)
	if err != nil {
		return nil, err
	}

	if len(ret) == 0 {
		return nil, sql.ErrNoRows
	}

	return ret[0], nil
}

func (qb *StoryStore) getMany(ctx context.Context, q *goqu.SelectDataset) ([]*models.Story, error) {
	q = librarylimit.Apply(ctx, q, librarylimit.Stories)

	const single = false
	var ret []*models.Story
	var lastID int
	if err := queryFunc(ctx, q, single, func(r *sqlx.Rows) error {
		var f storyQueryRow
		if err := r.StructScan(&f); err != nil {
			return err
		}

		s := f.resolve()
		if s.ID == lastID {
			return fmt.Errorf("internal error: multiple rows returned for single story id %d", s.ID)
		}
		lastID = s.ID

		ret = append(ret, s)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := qb.loadUserData(ctx, ret); err != nil {
		return nil, err
	}

	return ret, nil
}

func (qb *StoryStore) loadUserData(ctx context.Context, stories []*models.Story) error {
	if _, ok := models.UserIDFromContext(ctx); !ok || len(stories) == 0 {
		return nil
	}

	byID := make(map[int]*models.Story, len(stories))
	ids := make([]int, len(stories))
	for i, a := range stories {
		byID[a.ID] = a
		ids[i] = a.ID
	}

	return batchExec(ids, defaultBatchSize, func(batch []int) error {
		q := storyUserData.selectForUser(ctx, batch)
		const single = false
		return queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
			var r storyUserDataRow
			if err := rows.StructScan(&r); err != nil {
				return err
			}

			if a := byID[r.StoryID]; a != nil {
				r.apply(a)
			}
			return nil
		})
	})
}

func (qb *StoryStore) GetManyFileIDs(ctx context.Context, ids []int) ([][]models.FileID, error) {
	const primaryOnly = false
	return storyRepository.files.getMany(ctx, ids, primaryOnly)
}

func (qb *StoryStore) FindByFileID(ctx context.Context, fileID models.FileID) ([]*models.Story, error) {
	sq := dialect.From(storiesFilesJoinTable).Select(storiesFilesJoinTable.Col(storyIDColumn)).Where(
		storiesFilesJoinTable.Col(fileIDColumn).Eq(fileID),
	)

	ret, err := qb.findBySubquery(ctx, sq)
	if err != nil {
		return nil, fmt.Errorf("getting stories by file id %d: %w", fileID, err)
	}

	return ret, nil
}

func (qb *StoryStore) GetManyIDsByFileIDs(ctx context.Context, fileIDs []models.FileID) ([][]int, error) {
	sq := dialect.From(storiesFilesJoinTable).Select(storiesFilesJoinTable.Col(storyIDColumn), storiesFilesJoinTable.Col(fileIDColumn)).Where(
		storiesFilesJoinTable.Col(fileIDColumn).In(fileIDs),
	)

	sql, args, err := sq.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building query: %w", err)
	}

	var results []struct {
		StoryID int           `db:"story_id"`
		FileID  models.FileID `db:"file_id"`
	}

	if err := querySelect(ctx, sql, args, &results); err != nil {
		return nil, fmt.Errorf("getting stories by file ids %v: %w", fileIDs, err)
	}

	retMap := make(map[models.FileID][]int)
	for _, r := range results {
		retMap[r.FileID] = append(retMap[r.FileID], r.StoryID)
	}

	ret := make([][]int, len(fileIDs))
	for i, id := range fileIDs {
		ret[i] = retMap[id]
	}

	return ret, nil
}

func (qb *StoryStore) FindByPrimaryFileID(ctx context.Context, fileID models.FileID) ([]*models.Story, error) {
	sq := dialect.From(storiesFilesJoinTable).Select(storiesFilesJoinTable.Col(storyIDColumn)).Where(
		storiesFilesJoinTable.Col(fileIDColumn).Eq(fileID),
		storiesFilesJoinTable.Col("primary").IsTrue(),
	)

	ret, err := qb.findBySubquery(ctx, sq)
	if err != nil {
		return nil, fmt.Errorf("getting stories by primary file id %d: %w", fileID, err)
	}

	return ret, nil
}

func (qb *StoryStore) CountByFileID(ctx context.Context, fileID models.FileID) (int, error) {
	joinTable := storiesFilesJoinTable

	q := dialect.Select(goqu.COUNT("*")).From(joinTable).Where(joinTable.Col(fileIDColumn).Eq(fileID))
	return count(ctx, q)
}

func (qb *StoryStore) Count(ctx context.Context) (int, error) {
	q := dialect.Select(goqu.COUNT("*")).From(qb.table())
	return count(ctx, q)
}

func (qb *StoryStore) QueryCount(ctx context.Context, storyFilter *models.StoryFilterType, findFilter *models.FindFilterType) (int, error) {
	query, err := qb.makeQuery(ctx, storyFilter, findFilter)
	if err != nil {
		return 0, err
	}

	return query.executeCount(ctx)
}

func (qb *StoryStore) GetURLs(ctx context.Context, storyID int) ([]string, error) {
	return storiesURLsTableMgr.get(ctx, storyID)
}

func (qb *StoryStore) AssignFiles(ctx context.Context, storyID int, fileIDs []models.FileID) error {
	// assuming a file can only be assigned to a single story
	if err := storiesFilesTableMgr.destroyJoins(ctx, fileIDs); err != nil {
		return err
	}

	// assign primary only if destination has no files
	existingFileIDs, err := storyRepository.files.get(ctx, storyID)
	if err != nil {
		return err
	}

	firstPrimary := len(existingFileIDs) == 0
	return storiesFilesTableMgr.insertJoins(ctx, storyID, firstPrimary, fileIDs)
}

func (qb *StoryStore) AddFileID(ctx context.Context, id int, fileID models.FileID) error {
	const firstPrimary = false
	return storiesFilesTableMgr.insertJoins(ctx, id, firstPrimary, []models.FileID{fileID})
}

func (qb *StoryStore) GetTagIDs(ctx context.Context, id int) ([]int, error) {
	return storyRepository.tags.getIDs(ctx, id)
}
func (qb *StoryStore) GetFiles(ctx context.Context, id int) ([]models.File, error) {
	ids, err := storyRepository.files.get(ctx, id)
	if err != nil {
		return nil, err
	}
	files, err := qb.repo.File.Find(ctx, ids...)
	if err != nil {
		return nil, err
	}

	// never nil, so a story with no files still counts as loaded
	ret := make([]models.File, len(files))
	copy(ret, files)

	return ret, nil
}

func (qb *StoryStore) makeQuery(ctx context.Context, filter *models.StoryFilterType, find *models.FindFilterType) (*queryBuilder, error) {
	if filter == nil {
		filter = &models.StoryFilterType{}
	}
	if find == nil {
		find = &models.FindFilterType{}
	}
	q := storyRepository.newQuery()
	if where, ok := librarylimit.Where(ctx, librarylimit.Stories); ok {
		q.addWhere(where)
	}
	distinctIDs(&q, storyTable)
	if find.Q != nil && *find.Q != "" {
		q.addJoins(
			join{table: storiesFilesTable, onClause: "stories_files.story_id = stories.id"},
			join{table: fileTable, onClause: "stories_files.file_id = files.id"},
			join{table: folderTable, onClause: "files.parent_folder_id = folders.id"},
		)
		pathColumn := "folders.path || '" + string(filepath.Separator) + "' || files.basename"
		q.parseQueryString([]string{"stories.title", "stories.author", "stories.source_website", "stories.language", "stories.details", "stories.tagline", pathColumn}, *find.Q)
	}
	if err := q.addFilter(filterBuilderFromHandler(ctx, &storyFilterHandler{filter})); err != nil {
		return nil, err
	}
	sort := find.GetSort("title")
	if err := (sortOptions{"id", "title", "author", "date", "created_at", "updated_at", "rating"}).validateSort(sort); err != nil {
		return nil, err
	}
	sortTable := storyTable
	if sort == "rating" {
		storyUserData.joinSort(ctx, &q)
		sortTable = storiesUserDataTable
	}
	add, group := getSort(sort, find.GetDirection(), sortTable)
	q.sort = add
	q.addGroupBy(group...)
	q.sort += ", stories.id ASC"
	q.addGroupBy("stories.id")
	q.pagination = getPagination(find)
	return &q, nil
}
func (qb *StoryStore) Query(ctx context.Context, options models.StoryQueryOptions) (*models.StoryQueryResult, error) {
	q, err := qb.makeQuery(ctx, options.StoryFilter, options.FindFilter)
	if err != nil {
		return nil, err
	}
	ret := models.NewStoryQueryResult(qb)
	if options.Count {
		ret.Count, err = q.executeCount(ctx)
		if err != nil {
			return nil, err
		}
	}
	ret.IDs, err = q.findIDs(ctx)
	return ret, err
}
