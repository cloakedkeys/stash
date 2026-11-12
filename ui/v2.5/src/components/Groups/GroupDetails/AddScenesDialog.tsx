import React, { useState } from "react";
import { Form } from "react-bootstrap";
import { useIntl } from "react-intl";
import { faPlus } from "@fortawesome/free-solid-svg-icons";
import * as GQL from "src/core/generated-graphql";
import {
  queryFindScenesByID,
  useBulkSceneUpdate,
  useSceneUpdate,
} from "src/core/StashService";
import { Scene, SceneSelect } from "src/components/Scenes/SceneSelect";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import { NumberField } from "src/utils/form";

export const AddScenesDialog: React.FC<{
  group: GQL.GroupDataFragment;
  onClose: (applied: boolean) => void;
}> = ({ group, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [scenes, setScenes] = useState<Scene[]>([]);
  const [startingNumber, setStartingNumber] = useState("");
  const [invalid, setInvalid] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [updateScenes] = useBulkSceneUpdate();
  const [updateScene] = useSceneUpdate();

  async function onSave() {
    if (isUpdating || invalid || !scenes.length) return;
    setIsUpdating(true);
    try {
      // The picker does not load memberships. Read them at save time, then
      // restore pick order because the query can return a different order.
      const { data } = await queryFindScenesByID(
        scenes.map((s) => Number(s.id))
      );
      const byId = new Map(data.findScenes.scenes.map((s) => [s.id, s]));
      const selected = scenes.map((s) => {
        const scene = byId.get(s.id);
        if (!scene)
          throw new Error(
            intl.formatMessage({ id: "group_add_scenes.missing_scene" })
          );
        return scene;
      });
      const additions = selected.filter(
        (s) => !s.groups.some((g) => g.group.id === group.id)
      );

      if (startingNumber === "") {
        if (additions.length) {
          await updateScenes({
            variables: {
              input: {
                ids: additions.map((s) => s.id),
                group_ids: { ids: [group.id], mode: GQL.BulkUpdateIdMode.Add },
              },
            },
          });
        }
      } else {
        const start = Number(startingNumber);
        if (start + additions.length - 1 > 2147483647) {
          throw new Error(
            intl.formatMessage({ id: "group_add_scenes.invalid_number" })
          );
        }
        for (const [index, scene] of additions.entries()) {
          await updateScene({
            variables: {
              input: {
                id: scene.id,
                groups: [
                  ...scene.groups.map((g) => ({
                    group_id: g.group.id,
                    scene_index: g.scene_index,
                  })),
                  { group_id: group.id, scene_index: start + index },
                ],
              },
            },
          });
        }
      }
      Toast.success(
        intl.formatMessage(
          { id: "toast.scenes_added_to_group" },
          {
            added: additions.length,
            existing: selected.length - additions.length,
          }
        )
      );
      onClose(true);
    } catch (e) {
      Toast.error(e);
    } finally {
      setIsUpdating(false);
    }
  }

  return (
    <ModalComponent
      show
      icon={faPlus}
      header={intl.formatMessage({ id: "actions.add_scenes" })}
      accept={{
        onClick: onSave,
        text: intl.formatMessage({ id: "actions.add" }),
      }}
      cancel={{
        onClick: () => onClose(false),
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
      }}
      isRunning={isUpdating}
      disabled={invalid || !scenes.length}
    >
      <Form onSubmit={(e) => e.preventDefault()}>
        <Form.Group>
          <Form.Label>{intl.formatMessage({ id: "scenes" })}</Form.Label>
          <SceneSelect
            isMulti
            values={scenes}
            onSelect={setScenes}
            isDisabled={isUpdating}
            menuPortalTarget={document.body}
          />
        </Form.Group>
        <Form.Group controlId="starting-scene-number">
          <Form.Label>
            {intl.formatMessage({ id: "group_add_scenes.starting_number" })}
          </Form.Label>
          <NumberField
            value={startingNumber}
            step={1}
            min={-2147483648}
            max={2147483647}
            disabled={isUpdating}
            isInvalid={invalid}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
              const value = e.currentTarget.value;
              setStartingNumber(value);
              setInvalid(
                e.currentTarget.validity.badInput ||
                  (value !== "" &&
                    (!/^[+-]?\d+$/.test(value) ||
                      !Number.isInteger(Number(value)) ||
                      Number(value) < -2147483648 ||
                      Number(value) > 2147483647))
              );
            }}
          />
          <Form.Control.Feedback type="invalid">
            {intl.formatMessage({ id: "group_add_scenes.invalid_number" })}
          </Form.Control.Feedback>
          <Form.Text>
            {intl.formatMessage({ id: "group_add_scenes.number_help" })}
          </Form.Text>
        </Form.Group>
      </Form>
    </ModalComponent>
  );
};
