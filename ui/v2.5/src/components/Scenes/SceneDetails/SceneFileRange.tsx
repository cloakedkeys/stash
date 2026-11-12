import React, { useId } from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import { DurationInput } from "src/components/Shared/DurationInput";
import { getPlayerPosition } from "src/components/ScenePlayer/util";
import * as GQL from "src/core/generated-graphql";
import TextUtils from "src/utils/text";

interface IRangeFieldsProps {
  // Start of the scene currently loaded in the player, not the edited value.
  playerStartTime?: number | null;
  startTime: number | null | undefined;
  endTime: number | null | undefined;
  onStartTimeChange: (value: number | null) => void;
  onEndTimeChange: (value: number | null) => void;
}

export const SceneFileRangeFields: React.FC<IRangeFieldsProps> = ({
  playerStartTime,
  startTime,
  endTime,
  onStartTimeChange,
  onEndTimeChange,
}) => {
  const id = useId();

  function renderTime(
    field: "start_time" | "end_time",
    value: number | null | undefined,
    onChange: (value: number | null) => void
  ) {
    return (
      <Form.Group controlId={`${id}-${field}`}>
        <Form.Label>
          <FormattedMessage id={`scene_file_range.${field}`} />
        </Form.Label>
        <DurationInput value={value} setValue={onChange} />
        <Button
          variant="secondary"
          type="button"
          onClick={() => {
            const position = getPlayerPosition();
            if (position !== undefined)
              onChange((playerStartTime ?? 0) + position);
          }}
        >
          <FormattedMessage id="scene_file_range.set_from_current_time" />
        </Button>
      </Form.Group>
    );
  }

  return (
    <>
      {renderTime("start_time", startTime, onStartTimeChange)}
      {renderTime("end_time", endTime, onEndTimeChange)}
      <Form.Text className="text-muted mb-3">
        <FormattedMessage id="scene_file_range.help" />
      </Form.Text>
    </>
  );
};

export const SceneFileRangeDescription: React.FC<{
  scene: Pick<GQL.SceneDataFragment, "files" | "start_time" | "end_time">;
}> = ({ scene }) => {
  if (
    (scene.start_time == null && scene.end_time == null) ||
    !scene.files.length
  ) {
    return null;
  }

  return (
    <p className="px-3">
      <FormattedMessage
        id={
          scene.end_time == null
            ? "scene_file_range.part_to_end"
            : "scene_file_range.part"
        }
        values={{
          file: TextUtils.fileNameFromPath(scene.files[0].path),
          start: TextUtils.secondsToTimestamp(scene.start_time ?? 0),
          end:
            scene.end_time == null
              ? ""
              : TextUtils.secondsToTimestamp(scene.end_time),
        }}
      />
    </p>
  );
};
