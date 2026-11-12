import React from "react";
import * as GQL from "src/core/generated-graphql";
import {
  GroupsCriterion,
  GroupsCriterionOption,
} from "src/models/list-filter/criteria/groups";
import { ListFilterModel } from "src/models/list-filter/filter";
import { FilteredSceneList } from "src/components/Scenes/SceneList";
import { View } from "src/components/List/views";
import {
  queryFindScenes,
  useBulkSceneUpdate,
  useScenesUpdate,
} from "src/core/StashService";
import { IItemListOperation } from "src/components/List/FilteredListToolbar";
import {
  showWhenNoneSelected,
  showWhenSelected,
} from "src/components/List/ItemList";
import { AlertModal } from "src/components/Shared/Alert";
import { faMinus, faPlus } from "@fortawesome/free-solid-svg-icons";
import { useIntl } from "react-intl";
import { useToast } from "src/hooks/Toast";
import { useModal } from "src/hooks/modal";
import { AddScenesDialog } from "./AddScenesDialog";

interface IGroupScenesPanel {
  active: boolean;
  group: GQL.GroupDataFragment;
  showSubGroupContent?: boolean;
}

function useFilterHook(
  group: Pick<GQL.GroupDataFragment, "id" | "name">,
  showSubGroupContent?: boolean
) {
  return (filter: ListFilterModel) => {
    const groupValue = { id: group.id, label: group.name };
    // if group is already present, then we modify it, otherwise add
    let groupCriterion = filter.criteria.find((c) => {
      return c.criterionOption.type === "groups";
    }) as GroupsCriterion | undefined;

    if (
      groupCriterion &&
      (groupCriterion.modifier === GQL.CriterionModifier.IncludesAll ||
        groupCriterion.modifier === GQL.CriterionModifier.Includes)
    ) {
      // add the group if not present
      if (
        !groupCriterion.value.items.find((p) => {
          return p.id === group.id;
        })
      ) {
        groupCriterion.value.items.push(groupValue);
      }

      groupCriterion.modifier = GQL.CriterionModifier.IncludesAll;
    } else {
      // overwrite
      groupCriterion = new GroupsCriterion(GroupsCriterionOption);
      groupCriterion.value = {
        items: [groupValue],
        depth: showSubGroupContent ? -1 : 0,
        excluded: [],
      };
      filter.criteria.push(groupCriterion);
    }

    return filter;
  };
}

export const GroupScenesPanel: React.FC<IGroupScenesPanel> = ({
  active,
  group,
  showSubGroupContent,
}) => {
  const intl = useIntl();
  const Toast = useToast();
  const { modal, showModal, closeModal } = useModal();
  const [updateScenes] = useBulkSceneUpdate();
  const [renumberScenes] = useScenesUpdate([]);
  const directGroupFilter = useFilterHook(group);

  async function onMove(
    srcIds: string[],
    targetId: string,
    after: boolean,
    direction: GQL.SortDirectionEnum
  ) {
    if (!srcIds.some((id) => id !== "") || srcIds.includes(targetId)) return;
    // Read the whole direct group, regardless of the visible page or filters.
    // Each update replaces groups, so retain all of the other memberships.
    const filter = directGroupFilter(
      new ListFilterModel(GQL.FilterMode.Scenes)
    );
    filter.itemsPerPage = -1;
    filter.sortBy = "group_scene_number";
    filter.sortDirection = direction;
    const { data } = await queryFindScenes(filter);
    const scenes = data.findScenes.scenes;
    const byId = new Map(scenes.map((scene) => [scene.id, scene]));
    if (!byId.has(targetId) || srcIds.some((id) => !byId.has(id))) {
      throw new Error(
        intl.formatMessage({ id: "group_add_scenes.missing_scene" })
      );
    }
    const sequential = scenes.every(
      (scene, index) =>
        scene.groups.find((g) => g.group.id === group.id)?.scene_index ===
        (direction === GQL.SortDirectionEnum.Desc
          ? scenes.length - index
          : index + 1)
    );
    if (!sequential) {
      const confirmed = await new Promise<boolean>((resolve) => {
        showModal(
          <AlertModal
            show
            text={intl.formatMessage(
              { id: "dialogs.renumber_group_scenes" },
              { count: scenes.length }
            )}
            confirmButtonText={intl.formatMessage({ id: "actions.continue" })}
            onCancel={() => {
              closeModal();
              resolve(false);
            }}
            onConfirm={() => {
              closeModal();
              resolve(true);
            }}
          />
        );
      });
      if (!confirmed) return;
    }
    const movingIds = new Set(srcIds);
    const reordered = scenes.filter((scene) => !movingIds.has(scene.id));
    const at = reordered.findIndex((scene) => scene.id === targetId);
    reordered.splice(
      at + (after ? 1 : 0),
      0,
      ...srcIds.map((id) => byId.get(id)!)
    );
    const input = reordered.map((scene, index) => ({
      id: scene.id,
      groups: scene.groups.map((g) => ({
        group_id: g.group.id,
        scene_index:
          g.group.id === group.id
            ? direction === GQL.SortDirectionEnum.Desc
              ? scenes.length - index
              : index + 1
            : g.scene_index,
      })),
    }));
    await renumberScenes({ variables: { input } });
  }
  const filterHook = useFilterHook(group, showSubGroupContent);

  const extraOperations: IItemListOperation<GQL.FindScenesQueryResult>[] = [
    {
      requiredRole: GQL.RoleEnum.Modify,
      text: intl.formatMessage({ id: "actions.add_scenes" }),
      isDisplayed: showWhenNoneSelected,
      icon: faPlus,
      buttonVariant: "secondary",
      onClick: async (result) => {
        showModal(
          <AddScenesDialog
            group={group}
            onClose={(applied) => {
              closeModal();
              if (applied) {
                void result.refetch().catch((e) => Toast.error(e));
              }
            }}
          />
        );
      },
    },
    {
      requiredRole: GQL.RoleEnum.Modify,
      text: intl.formatMessage({ id: "actions.remove_from_group" }),
      isDisplayed: showWhenSelected,
      icon: faMinus,
      buttonVariant: "danger",
      onClick: async (result, _filter, selectedIds) => {
        const ids = Array.from(selectedIds);
        showModal(
          <AlertModal
            show
            text={intl.formatMessage(
              { id: "dialogs.remove_scenes_from_group" },
              { count: ids.length }
            )}
            onCancel={closeModal}
            onConfirm={async () => {
              closeModal();
              try {
                await updateScenes({
                  variables: {
                    input: {
                      ids,
                      group_ids: {
                        ids: [group.id],
                        mode: GQL.BulkUpdateIdMode.Remove,
                      },
                    },
                  },
                });
                Toast.success(
                  intl.formatMessage(
                    { id: "toast.removed_entity" },
                    {
                      count: ids.length,
                      singularEntity: intl.formatMessage({ id: "scene" }),
                      pluralEntity: intl.formatMessage({ id: "scenes" }),
                    }
                  )
                );
                await result.refetch();
              } catch (e) {
                Toast.error(e);
              }
            }}
          />
        );
      },
    },
  ];

  if (group?.id) {
    return (
      <>
        {modal}
        <FilteredSceneList
          filterHook={filterHook}
          defaultSort="group_scene_number"
          manualSortBy="group_scene_number"
          onMove={onMove}
          alterQuery={active}
          view={View.GroupScenes}
          fromGroupId={group.id}
          extraOperations={extraOperations}
        />
      </>
    );
  }
  return null;
};
