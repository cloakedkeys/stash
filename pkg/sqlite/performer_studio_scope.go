package sqlite

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

// Use the same expression for count filters and sorting, including zero counts.
func performerMediaCountSQL(ctx context.Context, mediaTable, joinTable, mediaFK string, scope *models.StudioScopeInput) (string, error) {
	query := fmt.Sprintf("(SELECT COUNT(*) FROM %s s", joinTable)
	if scope != nil {
		query += fmt.Sprintf(" INNER JOIN %s ON %s.id = s.%s", mediaTable, mediaTable, mediaFK)
	}
	query += " WHERE s.performer_id = performers.id"
	condition, err := performerStudioScopeSQL(ctx, mediaTable, scope)
	if err != nil {
		return "", err
	}
	return query + condition + ")", nil
}

func performerStudioScopeSQL(ctx context.Context, mediaTable string, scope *models.StudioScopeInput) (string, error) {
	if scope == nil {
		return "", nil
	}
	id, err := strconv.Atoi(scope.ID)
	if err != nil {
		return "", fmt.Errorf("invalid studio scope ID: %w", err)
	}
	values, err := getHierarchicalValues(ctx, []string{strconv.Itoa(id)}, studioTable, "", "parent_id", "child_id", scope.Depth)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(" AND %s.studio_id IN (SELECT column2 FROM (%s))", mediaTable, values), nil
}
