import React from "react";
import { useQuery } from "@apollo/client";
import { useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { objectTitle } from "src/core/files";
import {
  Criterion,
  ILabeledIdCriterion,
  IHierarchicalLabeledIdCriterion,
} from "src/models/list-filter/criteria/criterion";
import { PerformersCriterion } from "src/models/list-filter/criteria/performers";
import { ILabeledId } from "src/models/list-filter/types";

const queries = {
  tags: GQL.FindTagsForSelectDocument,
  scene_tags: GQL.FindTagsForSelectDocument,
  performer_tags: GQL.FindTagsForSelectDocument,
  performers: GQL.FindPerformersForSelectDocument,
  studios: GQL.FindStudiosForSelectDocument,
  groups: GQL.FindGroupsForSelectDocument,
  scenes: GQL.FindScenesForSelectDocument,
  galleries: GQL.FindGalleriesForSelectDocument,
};

type EntityCriterion =
  | ILabeledIdCriterion
  | IHierarchicalLabeledIdCriterion
  | PerformersCriterion;

type LabelQuery = Partial<
  GQL.FindTagsForSelectQuery &
    GQL.FindPerformersForSelectQuery &
    GQL.FindStudiosForSelectQuery &
    GQL.FindGroupsForSelectQuery &
    GQL.FindScenesForSelectQuery &
    GQL.FindGalleriesForSelectQuery
>;

const EntityCriterionLabel: React.FC<{
  criterion: EntityCriterion;
  query: (typeof queries)[keyof typeof queries];
  sfwContentMode?: boolean;
}> = ({ criterion, query, sfwContentMode }) => {
  const intl = useIntl();
  const value = criterion.value;
  const items = Array.isArray(value)
    ? value
    : [...value.items, ...(value.excluded ?? [])];
  const ids = [...new Set(items.map((item) => item.id))];
  const hasValue =
    criterion.modifier !== GQL.CriterionModifier.IsNull &&
    criterion.modifier !== GQL.CriterionModifier.NotNull;

  const { data } = useQuery<LabelQuery>(query, {
    variables: { ids },
    skip: !hasValue || ids.length === 0,
    fetchPolicy: "cache-and-network",
  });

  const namedObjects =
    data?.findTags?.tags ??
    data?.findPerformers?.performers ??
    data?.findStudios?.studios ??
    data?.findGroups?.groups;
  const titledObjects =
    data?.findScenes?.scenes ?? data?.findGalleries?.galleries;
  const labels = new Map(
    namedObjects?.map((item) => [item.id, item.name]) ??
      titledObjects?.map((item) => [item.id, objectTitle(item)])
  );
  const updateLabel = (item: ILabeledId): ILabeledId => ({
    ...item,
    label: labels.get(item.id) ?? item.label,
  });

  // Refresh display names without changing the saved filter or its selection.
  const displayCriterion: EntityCriterion = criterion.clone();
  if (displayCriterion instanceof ILabeledIdCriterion) {
    displayCriterion.value = displayCriterion.value.map(updateLabel);
  } else {
    displayCriterion.value = {
      ...displayCriterion.value,
      items: displayCriterion.value.items.map(updateLabel),
      excluded: (displayCriterion.value.excluded ?? []).map(updateLabel),
    };
  }

  return <>{displayCriterion.getLabel(intl, sfwContentMode)}</>;
};

export const CriterionLabel: React.FC<{
  criterion: Criterion;
  sfwContentMode?: boolean;
}> = ({ criterion, sfwContentMode }) => {
  const intl = useIntl();
  if (
    criterion instanceof ILabeledIdCriterion ||
    criterion instanceof IHierarchicalLabeledIdCriterion ||
    criterion instanceof PerformersCriterion
  ) {
    const { inputType } = criterion.modifierCriterionOption();
    if (inputType && inputType in queries) {
      return (
        <EntityCriterionLabel
          criterion={criterion}
          query={queries[inputType as keyof typeof queries]}
          sfwContentMode={sfwContentMode}
        />
      );
    }
  }

  return <>{criterion.getLabel(intl, sfwContentMode)}</>;
};
