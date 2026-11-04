package savedfilter

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
)

type ImporterReaderWriter interface {
	models.SavedFilterWriter
}

type Importer struct {
	ReaderWriter        ImporterReaderWriter
	Input               jsonschema.SavedFilter
	MissingRefBehaviour models.ImportMissingRefEnum

	savedFilter models.SavedFilter
}

func (i *Importer) PreImport(ctx context.Context) error {
	i.savedFilter = models.SavedFilter{
		Name:         i.Input.Name,
		Mode:         i.Input.Mode,
		FindFilter:   i.Input.FindFilter,
		ObjectFilter: i.Input.ObjectFilter,
		UIOptions:    i.Input.UIOptions,
	}

	if !i.Input.LiteralWildcards {
		i.convertLegacyWildcards()
	}

	return nil
}

// convertLegacyWildcards rewrites the text filters of a filter exported by a
// version where _ and % in them were wildcards, as the upgrade does for saved
// filters in the database.
func (i *Importer) convertLegacyWildcards() {
	desc := fmt.Sprintf("imported saved filter %q", i.savedFilter.Name)

	if f := i.savedFilter.FindFilter; f != nil && f.Q != nil {
		if note := searchNote(map[string]interface{}{"q": *f.Q}); note != "" {
			logger.Warnf("Left %s as it was: %s", desc, note)
		}
	}

	changed, notes := ConvertLegacyWildcards(i.savedFilter.Mode, i.savedFilter.ObjectFilter)
	for _, note := range notes {
		logger.Warnf("Left part of %s as it was: %s", desc, note)
	}
	if changed {
		logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
	}
}

func (i *Importer) PostImport(ctx context.Context, id int) error {
	return nil
}

func (i *Importer) Name() string {
	return i.Input.Name
}

func (i *Importer) FindExistingID(ctx context.Context) (*int, error) {
	// for now, assume this is only imported in full, so we don't support updating existing filters
	return nil, nil
}

func (i *Importer) Create(ctx context.Context) (*int, error) {
	err := i.ReaderWriter.Create(ctx, &i.savedFilter)
	if err != nil {
		return nil, fmt.Errorf("error creating saved filter: %v", err)
	}

	id := i.savedFilter.ID
	return &id, nil
}

func (i *Importer) Update(ctx context.Context, id int) error {
	return fmt.Errorf("updating existing saved filters is not supported")
}
