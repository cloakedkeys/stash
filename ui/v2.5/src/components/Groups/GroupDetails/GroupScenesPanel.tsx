import React from "react";
import * as GQL from "src/core/generated-graphql";
import {
  GroupsCriterion,
  GroupsCriterionOption,
} from "src/models/list-filter/criteria/groups";
import { ListFilterModel } from "src/models/list-filter/filter";
import { FilteredSceneList } from "src/components/Scenes/SceneList";
import { View } from "src/components/List/views";
import { useBulkSceneUpdate } from "src/core/StashService";
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
