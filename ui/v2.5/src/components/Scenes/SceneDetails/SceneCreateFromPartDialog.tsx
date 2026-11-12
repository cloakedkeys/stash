import React, { useState } from "react";
import { Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { useHistory } from "react-router-dom";
import { ModalComponent } from "src/components/Shared/Modal";
import { mutateCreateScene } from "src/core/StashService";
import { useToast } from "src/hooks/Toast";
import { SceneFileRangeFields } from "./SceneFileRange";

export const SceneCreateFromPartDialog: React.FC<{
  fileID: string;
  playerStartTime?: number | null;
  // the file's length in seconds, when known
  fileDuration?: number;
  sceneTitle: string;
  onClose: () => void;
}> = ({ fileID, fileDuration, playerStartTime, sceneTitle, onClose }) => {
  const intl = useIntl();
  const history = useHistory();
  const Toast = useToast();
  const [title, setTitle] = useState(
    intl.formatMessage(
      { id: "scene_file_range.part_title" },
      { title: sceneTitle }
    )
  );
  const [startTime, setStartTime] = useState<number | null>(null);
  const [endTime, setEndTime] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);

  // With no start and no end before the file's end, the server would make an
  // ordinary whole-file scene and take the file from its current scene.
  const endsEarly =
    endTime != null &&
    !(fileDuration && fileDuration > 0 && endTime === fileDuration);
  const makesPart = (startTime ?? 0) > 0 || endsEarly;

  async function onCreate() {
    setSaving(true);
    try {
      const result = await mutateCreateScene({
        file_ids: [fileID],
        title,
        start_time: startTime,
        end_time: endTime,
      });
      if (result.data?.sceneCreate) {
        history.push(`/scenes/${result.data.sceneCreate.id}`);
        onClose();
      }
    } catch (e) {
      Toast.error(e);
    } finally {
      setSaving(false);
    }
  }

  return (
    <ModalComponent
      show
      header={intl.formatMessage({ id: "scene_file_range.create" })}
      onHide={onClose}
      isRunning={saving}
      disabled={!makesPart}
      cancel={{ onClick: onClose }}
      accept={{
        text: intl.formatMessage({ id: "actions.create" }),
        onClick: onCreate,
      }}
    >
      <Form.Group controlId="part_title">
        <Form.Label>
          <FormattedMessage id="title" />
        </Form.Label>
        <Form.Control
          className="text-input"
          value={title}
          onChange={(e) => setTitle(e.currentTarget.value)}
        />
      </Form.Group>
      <SceneFileRangeFields
        playerStartTime={playerStartTime}
        startTime={startTime}
        endTime={endTime}
        onStartTimeChange={setStartTime}
        onEndTimeChange={setEndTime}
      />
    </ModalComponent>
  );
};
