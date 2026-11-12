import React, { useState } from "react";
import { Button, Form, Table } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import type * as GQL from "src/core/generated-graphql";
import { useSceneUpdate } from "src/core/StashService";
import { useCanModify } from "src/hooks/CurrentUser";
import { useToast } from "src/hooks/Toast";
import { NumberField } from "src/utils/form";
import { objectTitle } from "src/core/files";

interface Draft {
  scene: GQL.SlimSceneDataFragment;
  value: string;
  invalid: boolean;
}

export const GroupSceneNumbers: React.FC<
  React.PropsWithChildren<{
    scenes: GQL.SlimSceneDataFragment[];
    groupId?: string;
    onSaved?: () => Promise<unknown>;
  }>
> = ({ children, scenes, groupId, onSaved }) => {
  const intl = useIntl();
  const canModify = useCanModify();
  const Toast = useToast();
  const [updateScene] = useSceneUpdate();
  const [drafts, setDrafts] = useState<Draft[] | null>(null);
  const [saving, setSaving] = useState(false);
  const members = scenes.filter((s) =>
    s.groups.some((g) => g.group.id === groupId)
  );

  function startEditing() {
    setDrafts(
      members.map((scene) => ({
        scene,
        value: String(
          scene.groups.find((g) => g.group.id === groupId)?.scene_index ?? ""
        ),
        invalid: false,
      }))
    );
  }

  async function save() {
    if (!drafts || saving || drafts.some((d) => d.invalid)) return;
    setSaving(true);
    try {
      for (const draft of drafts) {
        const number = draft.value === "" ? null : Number(draft.value);
        const original =
          draft.scene.groups.find((g) => g.group.id === groupId)?.scene_index ??
          null;
        if (original === number) continue;
        await updateScene({
          variables: {
            input: {
              id: draft.scene.id,
              groups: draft.scene.groups.map((g) => ({
                group_id: g.group.id,
                scene_index: g.group.id === groupId ? number : g.scene_index,
              })),
            },
          },
        });
      }
      await onSaved?.();
      setDrafts(null);
    } catch (e) {
      Toast.error(e);
    } finally {
      setSaving(false);
    }
  }

  if (!canModify || !groupId || (!drafts && !members.length))
    return <>{children}</>;
  if (!drafts)
    return (
      <>
        <Button variant="secondary" className="mb-3" onClick={startEditing}>
          <FormattedMessage id="group_scene_numbers.edit" />
        </Button>
        {children}
      </>
    );

  return (
    <div>
      <p>
        <FormattedMessage id="group_scene_numbers.help" />
      </p>
      <Button className="mr-2" onClick={save} disabled={saving}>
        <FormattedMessage id="actions.save" />
      </Button>
      <Button
        variant="secondary"
        onClick={() => setDrafts(null)}
        disabled={saving}
      >
        <FormattedMessage id="actions.cancel" />
      </Button>
      <Table>
        <thead>
          <tr>
            <th>
              <FormattedMessage id="scene" />
            </th>
            <th>
              <FormattedMessage id="group_scene_number" />
            </th>
          </tr>
        </thead>
        <tbody>
          {drafts.map((draft, index) => (
            <tr key={draft.scene.id}>
              <td>{objectTitle(draft.scene)}</td>
              <td>
                <NumberField
                  aria-label={
                    intl.formatMessage({ id: "group_scene_number" }) +
                    ": " +
                    objectTitle(draft.scene)
                  }
                  aria-describedby={
                    draft.invalid
                      ? `scene-number-error-${draft.scene.id}`
                      : undefined
                  }
                  value={draft.value}
                  step={1}
                  min={-2147483648}
                  max={2147483647}
                  disabled={saving}
                  isInvalid={draft.invalid}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                    const value = e.currentTarget.value;
                    const invalid =
                      e.currentTarget.validity.badInput ||
                      (value !== "" &&
                        (!/^[+-]?\d+$/.test(value) ||
                          !Number.isInteger(Number(value)) ||
                          Number(value) < -2147483648 ||
                          Number(value) > 2147483647));
                    setDrafts(
                      drafts.map((d, i) =>
                        i === index ? { ...d, value, invalid } : d
                      )
                    );
                  }}
                />
                {draft.invalid && (
                  <Form.Control.Feedback
                    type="invalid"
                    id={`scene-number-error-${draft.scene.id}`}
                  >
                    <FormattedMessage id="group_scene_numbers.invalid" />
                  </Form.Control.Feedback>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
};
