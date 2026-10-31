import React, { useState } from "react";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import { useIntl } from "react-intl";
import { faSignOutAlt } from "@fortawesome/free-solid-svg-icons";
import { Col, Form, Row } from "react-bootstrap";
import * as FormUtils from "src/utils/form";
import { mutateAudioAssignFile } from "src/core/StashService";
import { Audio, AudioSelect } from "../AudioSelect";

interface IFile {
  id: string;
  path: string;
}

interface IReassignAudioFileDialogProps {
  selected: IFile;
  onClose: () => void;
}

export const ReassignAudioFileDialog: React.FC<
  IReassignAudioFileDialogProps
> = (props: IReassignAudioFileDialogProps) => {
  const [audios, setAudios] = useState<Audio[]>([]);

  const intl = useIntl();
  const singularEntity = intl.formatMessage({ id: "file" });
  const pluralEntity = intl.formatMessage({ id: "files" });

  const header = intl.formatMessage(
    { id: "dialogs.reassign_entity_title" },
    { count: 1, singularEntity, pluralEntity }
  );

  const toastMessage = intl.formatMessage(
    { id: "toast.reassign_past_tense" },
    { count: 1, singularEntity, pluralEntity }
  );

  const Toast = useToast();

  // Network state
  const [reassigning, setReassigning] = useState(false);

  async function onAccept() {
    if (!audios.length) {
      return;
    }

    setReassigning(true);
    try {
      await mutateAudioAssignFile(audios[0].id, props.selected.id);
      Toast.success(toastMessage);
      props.onClose();
    } catch (e) {
      Toast.error(e);
      props.onClose();
    }
    setReassigning(false);
  }

  return (
    <ModalComponent
      show
      icon={faSignOutAlt}
      header={header}
      accept={{
        onClick: onAccept,
        text: intl.formatMessage({ id: "actions.reassign" }),
      }}
      cancel={{
        onClick: () => props.onClose(),
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
      }}
      isRunning={reassigning}
    >
      <Form>
        <Form.Group controlId="dest" as={Row}>
          {FormUtils.renderLabel({
            title: intl.formatMessage({
              id: "dialogs.reassign_files.destination",
            }),
            labelProps: {
              column: true,
              sm: 3,
              xl: 12,
            },
          })}
          <Col sm={9} xl={12}>
            <AudioSelect
              values={audios}
              onSelect={(items) => setAudios(items)}
            />
          </Col>
        </Form.Group>
      </Form>
    </ModalComponent>
  );
};
