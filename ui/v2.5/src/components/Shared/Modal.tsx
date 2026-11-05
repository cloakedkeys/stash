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
  // Esc keeps cancelling after the user typed or changed something; for
  // search and picker dialogs. By default Esc stops once anything changed.
  escapeAfterChanges?: boolean;
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

// Controls that change a value when clicked: buttons (rating stars, add /
// remove / set toggles, scrape result choices), select and date picker
// options, and react-select's clear and remove crosses (aria-hidden or
// role="button").
const valueControls = [
  "button",
  "[role='button']",
  "[role='option']",
  "[role='checkbox']",
  "[role='radio']",
  "[role='switch']",
  "[aria-hidden='true']",
  ".dropdown-item",
].join(",");

function isPopupTrigger(target: Partial<Element>) {
  return target.getAttribute?.("aria-haspopup") === "true";
}

/**
 * Whether a click or key press in the dialog may have changed one of its
 * values. It errs towards yes: a click on a button that only opens a menu
 * counts too. The dialog's own header and footer buttons never count.
 */
function mayChangeValue(e: React.SyntheticEvent): boolean {
  const target = e.target as Partial<Element>;
  if (target.closest?.(".modal-header, .modal-footer")) {
    return false;
  }

  if (e.type === "keydown") {
    const { key } = e as React.KeyboardEvent;
    // picking an option in an open select by keyboard, or removing a value
    return (
      isPopupTrigger(target) &&
      (((key === "Enter" || key === "Tab") &&
        target.getAttribute?.("aria-expanded") === "true") ||
        key === "Backspace" ||
        key === "Delete")
    );
  }

  return !!target.closest?.(valueControls);
}

interface IEscapeToCancel {
  // what Esc runs: the dialog's Cancel action
  onCancel?: () => void;
  // Esc does nothing while true, e.g. while saving
  busy?: boolean;
  // Esc keeps cancelling after the user changed something. For search and
  // picker dialogs, where a typed query is nothing to lose.
  afterChanges?: boolean;
}

/**
 * Props for a react-bootstrap Modal that make Esc run the dialog's cancel
 * action - the same thing its Cancel button does. Esc does nothing:
 * - while the dialog is busy, or when it has no cancel action
 * - once the user has typed or changed anything in the dialog (unless
 *   `afterChanges`), so Esc never throws away their input; Cancel still does
 * - when it closes an open menu or date picker inside the dialog instead
 * - during IME composition.
 * Only the top dialog sees Esc. Esc never calls the Modal's onHide by itself.
 */
export function useEscapeToCancel({
  onCancel,
  busy,
  afterChanges,
}: IEscapeToCancel) {
  // Menus close themselves on Esc before the dialog sees it, so whether a
  // menu was open is read on the way down, before they handle the key.
  const keyDown = useRef<IEscapeKeyDown>();
  // set by the first input or change in the dialog, cleared when it reopens
  const changed = useRef(false);

  function markChanged() {
    changed.current = true;
  }

  function onKeyDownCapture(e: React.KeyboardEvent) {
    if (e.key === "Escape") {
      keyDown.current = readEscapeKeyDown(e);
    } else if (mayChangeValue(e)) {
      markChanged();
    }
  }

  function onPointerCapture(e: React.SyntheticEvent) {
    if (mayChangeValue(e)) {
      markChanged();
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

    if (
      handledInside ||
      busy ||
      !onCancel ||
      (changed.current && !afterChanges)
    ) {
      return;
    }

    onCancel();
  }

  return {
    keyboard: true,
    onShow: () => {
      changed.current = false;
    },
    onKeyDownCapture,
    // typing, pasting, checkboxes, native selects
    onInputCapture: markChanged,
    onChangeCapture: markChanged,
    // mousedown as well as click: react-select's crosses act on mousedown
    // and are gone by the click
    onMouseDownCapture: onPointerCapture,
    onClickCapture: onPointerCapture,
    onEscapeKeyDown,
  };
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
  escapeAfterChanges,
}) => {
  // Some dialogs have no Cancel button and close with the accept button
  // instead; that button is also disabled by `disabled`.
  const escapeAction = cancel ? cancel.onClick : onHide;
  const escapeProps = useEscapeToCancel({
    onCancel: escapeAction,
    busy: isRunning || (disabled && escapeAction === accept?.onClick),
    afterChanges: escapeAfterChanges,
  });

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
