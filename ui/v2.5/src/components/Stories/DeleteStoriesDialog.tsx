import React, { useState } from "react";
import { Form } from "react-bootstrap";
import { useStoriesDestroy } from "src/core/StashService";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import { useConfigurationContext } from "src/hooks/Config";
import { FormattedMessage, useIntl } from "react-intl";
import { faTrashAlt } from "@fortawesome/free-solid-svg-icons";

interface IStoryToDelete {
  id: string;
  files: { path: string }[];
}

interface IDeleteStoriesDialogProps {
  selected: IStoryToDelete[];
  onClose: (confirmed: boolean) => void;
}

// Removes stories, and their files if asked. Without that, a story whose file stays in a
// library comes back on the next scan.
export const DeleteStoriesDialog: React.FC<IDeleteStoriesDialogProps> = (
  props: IDeleteStoriesDialogProps
) => {
  const intl = useIntl();
  const singularEntity = intl.formatMessage({ id: "story" });
  const pluralEntity = intl.formatMessage({ id: "stories" });
  const count = props.selected.length;

  const header = intl.formatMessage(
    { id: "dialogs.delete_entity_title" },
    { count, singularEntity, pluralEntity }
  );
  const toastMessage = intl.formatMessage(
    { id: "toast.delete_past_tense" },
    { count, singularEntity, pluralEntity }
  );
  const message = intl.formatMessage(
    { id: "dialogs.delete_entity_desc" },
    { count, singularEntity, pluralEntity }
  );

  const { configuration: config } = useConfigurationContext();

  // a story scraped from a website may have no file
  const files = props.selected.flatMap((s) => s.files.map((f) => f.path));

  const [deleteFile, setDeleteFile] = useState<boolean>(
    config?.defaults.deleteFile ?? false
  );

  const Toast = useToast();
  const [deleteStories] = useStoriesDestroy({
    ids: props.selected.map((s) => s.id),
    delete_file: deleteFile && files.length > 0,
  });

  const [isDeleting, setIsDeleting] = useState(false);

  async function onDelete() {
    setIsDeleting(true);
    try {
      await deleteStories();
      Toast.success(toastMessage);
      props.onClose(true);
    } catch (e) {
      Toast.error(e);
      props.onClose(false);
    }
    setIsDeleting(false);
  }

  function maybeRenderDeleteFileAlert() {
    if (!deleteFile || files.length === 0) {
      return;
    }

    const deleteAlertId = config?.general.deleteTrashPath
      ? "dialogs.delete_alert_to_trash"
      : "dialogs.delete_alert";
    const fileEntity = {
      singularEntity: intl.formatMessage({ id: "file" }),
      pluralEntity: intl.formatMessage({ id: "files" }),
    };

    return (
      <div className="delete-dialog alert alert-danger text-break">
        <p className="font-weight-bold">
          <FormattedMessage
            values={{ count: files.length, ...fileEntity }}
            id={deleteAlertId}
          />
        </p>
        <ul>
          {files.slice(0, 5).map((s) => (
            <li key={s}>{s}</li>
          ))}
          {files.length > 5 && (
            <FormattedMessage
              values={{ count: files.length - 5, ...fileEntity }}
              id="dialogs.delete_object_overflow"
            />
          )}
        </ul>
      </div>
    );
  }

  return (
    <ModalComponent
      show
      icon={faTrashAlt}
      header={header}
      accept={{
        variant: "danger",
        onClick: onDelete,
        text: intl.formatMessage({ id: "actions.delete" }),
      }}
      cancel={{
        onClick: () => props.onClose(false),
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
      }}
      isRunning={isDeleting}
    >
      <p>{message}</p>
      {maybeRenderDeleteFileAlert()}
      {files.length > 0 && (
        <Form>
          <Form.Check
            id="delete-file"
            checked={deleteFile}
            label={intl.formatMessage({ id: "actions.delete_file" })}
            onChange={() => setDeleteFile(!deleteFile)}
          />
        </Form>
      )}
    </ModalComponent>
  );
};

export default DeleteStoriesDialog;
