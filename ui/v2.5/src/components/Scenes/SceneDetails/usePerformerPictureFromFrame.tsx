import { useRef, useState } from "react";
import { Button } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { usePerformerUpdate } from "src/core/StashService";
import { ModalComponent } from "src/components/Shared/Modal";
import { VIDEO_PLAYER_ID } from "src/components/ScenePlayer/util";
import { captureFrame } from "src/components/ScenePlayer/captureFrame";
import { useToast } from "src/hooks/Toast";

interface IScenePerformer {
  id: string;
  name: string;
  disambiguation?: string | null;
}

// Sets a performer's picture to the video's current frame (#1977). The frame
// is taken as the browser shows it, so a transcoded stream gives its own
// size. With one performer it saves straight away; with more, it asks which.
export function usePerformerPictureFromFrame(performers: IScenePerformer[]) {
  const intl = useIntl();
  const Toast = useToast();
  const [updatePerformer] = usePerformerUpdate();
  const [frame, setFrame] = useState<string>();
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);

  async function save(performer: IScenePerformer, image: string) {
    if (busy.current) return;
    busy.current = true;
    setSaving(true);
    try {
      await updatePerformer({
        variables: { input: { id: performer.id, image } },
      });
      setFrame(undefined);
      Toast.success(
        intl.formatMessage(
          { id: "toast.performer_picture_saved" },
          { performer: performer.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }

  function capture() {
    if (busy.current || !performers.length) return;
    let image: string;
    try {
      const video =
        document.getElementById(VIDEO_PLAYER_ID)?.querySelector("video") ??
        null;
      image = captureFrame(video);
    } catch {
      Toast.error(
        intl.formatMessage({ id: "performer_picture.frame_unavailable" })
      );
      return;
    }
    if (performers.length === 1) {
      save(performers[0], image);
    } else {
      setFrame(image);
    }
  }

  const close = () => {
    if (!busy.current) setFrame(undefined);
  };

  const dialog = frame ? (
    <ModalComponent
      show
      header={intl.formatMessage({ id: "actions.set_as_performer_image" })}
      onHide={close}
      accept={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: close,
      }}
      isRunning={saving}
    >
      <img
        className="w-100 mb-3"
        src={frame}
        alt={intl.formatMessage({ id: "performer_picture.current_frame" })}
      />
      <p>
        <FormattedMessage id="performer_picture.choose_performer" />
      </p>
      {performers.map((performer) => (
        <Button
          type="button"
          key={performer.id}
          className="mr-2 mb-2"
          disabled={saving}
          onClick={() => save(performer, frame)}
        >
          {performer.name}
          {performer.disambiguation ? ` (${performer.disambiguation})` : ""}
        </Button>
      ))}
    </ModalComponent>
  ) : null;

  return { capture, dialog };
}
