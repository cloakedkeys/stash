package group

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
)

var (
	ErrEmptyAlias      = errors.New("group alias must not be an empty string")
	ErrAliasNotTrimmed = errors.New("group alias contains spaces at the beginning or the end")
)

func (s *Service) validateCreate(ctx context.Context, group *models.Group) error {
	if err := validateName(group.Name); err != nil {
		return err
	}

	if group.Aliases.Loaded() {
		if err := ValidateAliases(ctx, group.Aliases.List()); err != nil {
			return err
		}
	}

	containingIDs := group.ContainingGroups.IDs()
	subIDs := group.SubGroups.IDs()

	if err := s.validateLinkedGroups(ctx, containingIDs); err != nil {
		return err
	}
	if err := s.validateLinkedGroups(ctx, subIDs); err != nil {
		return err
	}

	// a new group can only end up in a loop if it has both
	if len(containingIDs) > 0 && len(subIDs) > 0 {
		if err := s.lockHierarchy(ctx); err != nil {
			return err
		}
	}

	if err := s.validateGroupHierarchy(ctx, containingIDs, subIDs); err != nil {
		return err
	}

	return nil
}

// HierarchyLocker is a group store whose write transactions can run at the
// same time, as on PostgreSQL.
type HierarchyLocker interface {
	// LockGroupHierarchy makes the caller wait for any other transaction
	// holding the lock to end, and holds it until the caller's transaction
	// ends.
	LockGroupHierarchy(ctx context.Context) error
}

// lockHierarchy makes edits that may add containing or sub-group links take
// turns, from the loop check until they're saved. Otherwise two edits at once,
// "A contains B" and "B contains A", could each pass the check before the other
// is saved, and leave a loop (#2113). It is taken before the edit changes
// anything. SQLite stores don't need it: they write one transaction at a time.
func (s *Service) lockHierarchy(ctx context.Context) error {
	if l, ok := s.Repository.(HierarchyLocker); ok {
		return l.LockGroupHierarchy(ctx)
	}
	return nil
}

func (s *Service) validateUpdate(ctx context.Context, id int, partial models.GroupPartial) error {
	// get the existing group - ensure it exists
	existing, err := s.Repository.Find(ctx, id)
	if err != nil {
		return err
	}

	if existing == nil {
		return models.ErrNotFound
	}

	if partial.Name.Set {
		if err := validateName(partial.Name.Value); err != nil {
			return err
		}
	}

	if partial.Aliases != nil {
		if err := ValidateAliases(ctx, partial.Aliases.Values); err != nil {
			return err
		}
	}

	for _, d := range []*models.UpdateGroupDescriptions{partial.ContainingGroups, partial.SubGroups} {
		// removing a group that doesn't exist removes nothing, as upstream
		if d != nil && d.Mode != models.RelationshipUpdateModeRemove {
			if err := s.validateLinkedGroups(ctx, idsFromGroupDescriptions(d.Groups)); err != nil {
				return err
			}
		}
	}

	if err := s.validateUpdateGroupHierarchy(ctx, existing, partial.ContainingGroups, partial.SubGroups); err != nil {
		return err
	}

	return nil
}

// validateLinkedGroups refuses containing or sub-groups that don't exist, saying
// "group with id 999 not found" as FindMany does, and a group listed twice. The
// database refused both with its own foreign key or UNIQUE error.
func (s *Service) validateLinkedGroups(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	if err := validateNoRepeatedGroups(ids); err != nil {
		return err
	}
	_, err := s.Repository.FindMany(ctx, ids)
	return err
}

// validateNoRepeatedGroups refuses a group listed twice. Each group can be linked
// once, so the database refused the second with its own UNIQUE error.
func validateNoRepeatedGroups(ids []int) error {
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("group with id %d is listed more than once", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateName(n string) error {
	// ensure name is not empty
	if strings.TrimSpace(n) == "" {
		return ErrEmptyName
	}

	return nil
}

// only validate if aliases are not empty and trimmed
// no requirement for uniqueness or difference from group name here
// deduplication and exclusion are ensured at resolver level
// import bypasses resolver though.
func ValidateAliases(ctx context.Context, aliases []string) error {
	for _, a := range aliases {
		if err := validateName(a); err != nil {
			if errors.Is(err, ErrEmptyName) {
				return ErrEmptyAlias
			}
			return err
		}
		if strings.TrimSpace(a) != a {
			return ErrAliasNotTrimmed
		}
	}

	return nil
}

func (s *Service) validateGroupHierarchy(ctx context.Context, containingIDs []int, subIDs []int) error {
	// only need to validate if both are non-empty
	if len(containingIDs) == 0 || len(subIDs) == 0 {
		return nil
	}

	// ensure none of the containing groups are in the sub groups
	found, err := s.Repository.FindInAncestors(ctx, containingIDs, subIDs)
	if err != nil {
		return err
	}

	if len(found) > 0 {
		return ErrHierarchyLoop
	}

	return nil
}

func (s *Service) validateUpdateGroupHierarchy(ctx context.Context, existing *models.Group, containingGroups *models.UpdateGroupDescriptions, subGroups *models.UpdateGroupDescriptions) error {
	// no need to validate if there are no changes
	if containingGroups == nil && subGroups == nil {
		return nil
	}

	// lock before reading the group's links, so they're the latest
	if err := s.lockHierarchy(ctx); err != nil {
		return err
	}

	if err := existing.LoadContainingGroupIDs(ctx, s.Repository); err != nil {
		return err
	}
	existingContainingGroups := existing.ContainingGroups.List()

	if err := existing.LoadSubGroupIDs(ctx, s.Repository); err != nil {
		return err
	}
	existingSubGroups := existing.SubGroups.List()

	effectiveContainingGroups := existingContainingGroups
	if containingGroups != nil {
		effectiveContainingGroups = containingGroups.Apply(existingContainingGroups)
	}

	effectiveSubGroups := existingSubGroups
	if subGroups != nil {
		effectiveSubGroups = subGroups.Apply(existingSubGroups)
	}

	containingIDs := idsFromGroupDescriptions(effectiveContainingGroups)
	subIDs := idsFromGroupDescriptions(effectiveSubGroups)

	// ensure we haven't set the group as a subgroup of itself
	if slices.Contains(containingIDs, existing.ID) || slices.Contains(subIDs, existing.ID) {
		return ErrHierarchyLoop
	}

	return s.validateGroupHierarchy(ctx, containingIDs, subIDs)
}

func idsFromGroupDescriptions(v []models.GroupIDDescription) []int {
	return sliceutil.Map(v, func(g models.GroupIDDescription) int { return g.GroupID })
}
