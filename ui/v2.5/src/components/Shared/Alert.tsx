import { Button, Modal } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import { PatchComponent } from "src/patch";
import { useEscapeToCancel } from "./Modal";

export interface IAlertModalProps {
  text: JSX.Element | string;
  confirmVariant?: string;
  show?: boolean;
  confirmButtonText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export const AlertModal: React.FC<IAlertModalProps> = PatchComponent(
  "AlertModal",
  ({
    text,
    show,
    confirmVariant = "danger",
    confirmButtonText,
    onConfirm,
    onCancel,
  }) => {
    const escapeProps = useEscapeToCancel(onCancel);

    return (
      <Modal show={show} {...escapeProps}>
        <Modal.Body>{text}</Modal.Body>
        <Modal.Footer>
          <Button variant={confirmVariant} onClick={() => onConfirm()}>
            {confirmButtonText ?? <FormattedMessage id="actions.confirm" />}
          </Button>
          <Button variant="secondary" onClick={() => onCancel()}>
            <FormattedMessage id="actions.cancel" />
          </Button>
        </Modal.Footer>
      </Modal>
    );
  }
);
