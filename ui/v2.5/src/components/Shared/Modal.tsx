import React, { useRef } from "react";
import { Button, Modal, Spinner, ModalProps } from "react-bootstrap";
import { ButtonVariant } from "react-bootstrap/types";
import { Icon } from "./Icon";
import { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import { FormattedMessage } from "react-intl";

interface IButton {
  text?: string;
  variant?: ButtonVariant;
  onClick?: () => void;
}

interface IModal {
  show: boolean;
  onHide?: () => void;
  header?: JSX.Element | string;
  icon?: IconDefinition;
  cancel?: IButton;
  accept?: IButton;
  isRunning?: boolean;
  disabled?: boolean;
  modalProps?: ModalProps;
  dialogClassName?: string;
  footerButtons?: React.ReactNode;
  leftFooterButtons?: React.ReactNode;
}

const defaultOnHide = () => {};

interface IEscapeKeyDown {
  event: Event;
  // a react-select or dropdown menu was open where Esc was pressed
  inOpenMenu: boolean;
  // Esc was pressed on a closed react-select or dropdown toggle
  onClosedMenu: boolean;
}

/** True while an IME (Japanese, Chinese, ...) is composing text. */
export function isComposing(e: KeyboardEvent) {
  return e.isComposing || e.keyCode === 229;
}

function readEscapeKeyDown(e: React.KeyboardEvent): IEscapeKeyDown {
  const target = e.target as Partial<Element>;
  // react-select inputs and dropdown toggles both mark themselves as popup
  // triggers; an expanded section toggle is not a menu
  const expanded =
    target.getAttribute?.("aria-haspopup") === "true"
      ? target.getAttribute?.("aria-expanded")
      : undefined;
  return {
    event: e.nativeEvent,
    inOpenMenu:
      expanded === "true" || !!target.closest?.(".dropdown-menu.show"),
    onClosedMenu: expanded === "false",
  };
}

/**
 * Props for a react-bootstrap Modal that make Esc run the dialog's cancel
 * action - the same thing its Cancel button does. Esc does nothing while the
 * dialog is busy, when it has no cancel action, or when it closes an open menu
 * or date picker inside the dialog instead. Only the top dialog sees Esc.
 *
 * Esc never calls the Modal's onHide by itself.
 */
export function useEscapeToCancel(onCancel?: () => void, busy?: boolean) {
  // Menus close themselves on Esc before the dialog sees it, so whether a
  // menu was open is read on the way down, before they handle the key.
  const keyDown = useRef<IEscapeKeyDown>();

  function onKeyDownCapture(e: React.KeyboardEvent) {
    if (e.key === "Escape") {
      keyDown.current = readEscapeKeyDown(e);
    }
  }

  function onEscapeKeyDown(e: KeyboardEvent) {
    const seen = keyDown.current?.event === e ? keyDown.current : undefined;
    keyDown.current = undefined;

    // Esc during IME input cancels the composition, not the dialog
    if (isComposing(e)) {
      e.preventDefault();
      return;
    }

    // react-select and Bootstrap dropdowns claim Esc even with their menu
    // closed, so a claim only counts when something was open to close.
    const handledInside =
      seen?.inOpenMenu || (e.defaultPrevented && !seen?.onClosedMenu);

    // stops the Modal calling onHide
    e.preventDefault();

    if (handledInside || busy || !onCancel) {
      return;
    }

    onCancel();
  }

  return { keyboard: true, onKeyDownCapture, onEscapeKeyDown };
}

export const ModalComponent: React.FC<React.PropsWithChildren<IModal>> = ({
  children,
  show,
  icon,
  header,
  cancel,
  accept,
  onHide,
  isRunning,
  disabled,
  modalProps,
  dialogClassName,
  footerButtons,
  leftFooterButtons,
}) => {
  // Some dialogs have no Cancel button and close with the accept button
  // instead; that button is also disabled by `disabled`.
  const escapeAction = cancel ? cancel.onClick : onHide;
  const escapeProps = useEscapeToCancel(
    escapeAction,
    isRunning || (disabled && escapeAction === accept?.onClick)
  );

  return (
    <Modal
      className="ModalComponent"
      onHide={onHide ?? defaultOnHide}
      show={show}
      dialogClassName={dialogClassName}
      {...escapeProps}
      {...modalProps}
    >
      <Modal.Header>
        {icon ? <Icon icon={icon} /> : ""}
        <span>{header ?? ""}</span>
      </Modal.Header>
      <Modal.Body>{children}</Modal.Body>
      <Modal.Footer className="ModalFooter">
        <div>{leftFooterButtons}</div>
        <div>
          {footerButtons}
          {cancel ? (
            <Button
              disabled={isRunning}
              variant={cancel.variant ?? "primary"}
              onClick={cancel.onClick}
              className="ml-2"
            >
              {cancel.text ?? (
                <FormattedMessage
                  id="actions.cancel"
                  defaultMessage="Cancel"
                  description="Cancels the current action and dismisses the modal."
                />
              )}
            </Button>
          ) : (
            ""
          )}
          <Button
            disabled={isRunning || disabled}
            variant={accept?.variant ?? "primary"}
            onClick={accept?.onClick}
            className="ml-2"
          >
            {isRunning ? (
              <Spinner animation="border" role="status" size="sm" />
            ) : (
              (accept?.text ?? (
                <FormattedMessage
                  id="actions.close"
                  defaultMessage="Close"
                  description="Closes the current modal."
                />
              ))
            )}
          </Button>
        </div>
      </Modal.Footer>
    </Modal>
  );
};
