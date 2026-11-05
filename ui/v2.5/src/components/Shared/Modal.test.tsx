import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { isComposing, ModalComponent } from "./Modal";
import { AlertModal } from "./Alert";
import { LoadFilterDialog, SaveFilterDialog } from "../List/SavedFilterList";
import { ReleaseNotesDialog } from "../Dialogs/ReleaseNotesDialog";
import FieldSelector from "../Tagger/FieldSelector";
import { hideSidebarOnEscape } from "../List/Filters/FilterSidebar";

// The props each react-bootstrap Modal was last rendered with.
type ModalProps = {
  show?: boolean;
  keyboard?: boolean;
  backdrop?: boolean | "static";
  onHide?: () => void;
  onEscapeKeyDown?: (e: KeyboardEvent) => void;
  onKeyDownCapture?: (e: Partial<React.KeyboardEvent>) => void;
};
const modals = vi.hoisted(() => ({ props: [] as unknown[] }));

vi.mock("react-bootstrap", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-bootstrap")>();
  const Part = ({ children }: React.PropsWithChildren) => <>{children}</>;
  return {
    ...original,
    Modal: Object.assign(
      (props: React.PropsWithChildren<ModalProps>) => {
        modals.props.push(props);
        return <div>{props.children}</div>;
      },
      { Header: Part, Title: Part, Body: Part, Footer: Part }
    ),
  };
});

vi.mock("./MarkdownPage", () => ({ MarkdownPage: () => null }));

vi.mock("src/core/StashService", () => ({
  useFindSavedFilters: () => ({ data: undefined }),
}));

function render(node: React.ReactNode): ModalProps {
  modals.props = [];
  renderToStaticMarkup(
    <IntlProvider locale="en" messages={{}} onError={() => {}}>
      {node}
    </IntlProvider>
  );
  expect(modals.props).toHaveLength(1);
  return modals.props[0] as ModalProps;
}

// An element the key is pressed on: a react-select input or dropdown toggle
// (a popup trigger, open or not), something inside an open dropdown menu, or
// a plain button.
type Target =
  | { popup: "open" | "closed" }
  | { inOpenDropdownMenu: true }
  | undefined;

function element(target: Target): Partial<Element> {
  const attributes: Record<string, string> = {};
  if (target && "popup" in target) {
    attributes["aria-haspopup"] = "true";
    attributes["aria-expanded"] = target.popup === "open" ? "true" : "false";
  }
  return {
    getAttribute: (name: string) => attributes[name] ?? null,
    closest: ((selector: string) =>
      target &&
      "inOpenDropdownMenu" in target &&
      selector.includes(".dropdown-menu.show")
        ? ({} as Element)
        : null) as Element["closest"],
  };
}

/**
 * Presses Esc in a dialog the way the browser and react-bootstrap 1.6 deliver
 * it: React's capture handlers on the dialog, then the handlers of whatever has
 * focus (`inside`), then the document keydown listener of the top Modal -
 * react-bootstrap's handleEscapeKeyDown followed by react-overlays, which calls
 * onHide unless the event was default-prevented.
 *
 * This copies react-bootstrap 1.6.8 cjs/Modal.js handleEscapeKeyDown and
 * react-overlays 5.2.1 cjs/Modal.js handleDocumentKeyDown; re-check it when
 * either is upgraded.
 *
 * `composing` presses Esc while an IME is composing text, as a browser reports
 * it with keyCode 27 (some report keyCode 229, which react-overlays ignores).
 */
function pressEscape(
  props: ModalProps,
  target?: Target,
  inside?: (e: KeyboardEvent) => void,
  composing = false
) {
  const fake = {
    key: "Escape",
    keyCode: 27,
    isComposing: composing,
    defaultPrevented: false,
    preventDefault: () => {
      fake.defaultPrevented = true;
    },
  };
  const event = fake as unknown as KeyboardEvent;

  props.onKeyDownCapture?.({
    key: "Escape",
    target: element(target) as EventTarget,
    nativeEvent: event,
  });
  inside?.(event);

  const keyboard = props.keyboard ?? true;
  if (!keyboard && props.backdrop === "static") {
    event.preventDefault();
  } else if (keyboard && props.onEscapeKeyDown) {
    props.onEscapeKeyDown(event);
  }
  if (!event.defaultPrevented) {
    props.onHide?.();
  }
}

// react-select and Bootstrap dropdowns claim Esc whether or not a menu is open
const claimsEscape = (e: KeyboardEvent) => e.preventDefault();

describe("Esc in a dialog (#1113)", () => {
  const cancel = vi.fn();
  const hide = vi.fn();
  const accept = vi.fn();

  beforeEach(() => vi.clearAllMocks());

  it("runs the Cancel button's action", () => {
    const props = render(
      <ModalComponent
        show
        cancel={{ onClick: cancel }}
        accept={{ onClick: accept }}
      />
    );
    pressEscape(props);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(accept).not.toHaveBeenCalled();
  });

  it("runs Cancel, not onHide, when the dialog has both", () => {
    const props = render(
      <ModalComponent
        show
        onHide={hide}
        cancel={{ onClick: cancel }}
        accept={{ onClick: accept }}
      />
    );
    pressEscape(props);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(hide).not.toHaveBeenCalled();
  });

  it("closes a dialog without a Cancel button through its onHide", () => {
    const props = render(
      <ModalComponent show onHide={hide} accept={{ onClick: hide }} />
    );
    pressEscape(props);
    expect(hide).toHaveBeenCalledTimes(1);
  });

  it("does nothing while the dialog is running", () => {
    for (const node of [
      <ModalComponent
        key="cancel"
        show
        isRunning
        cancel={{ onClick: cancel }}
        accept={{ onClick: accept }}
      />,
      <ModalComponent
        key="hide"
        show
        isRunning
        onHide={hide}
        accept={{ onClick: accept }}
      />,
    ]) {
      pressEscape(render(node));
    }
    expect(cancel).not.toHaveBeenCalled();
    expect(hide).not.toHaveBeenCalled();
    expect(accept).not.toHaveBeenCalled();
  });

  it("does nothing while its close button is disabled", () => {
    // ScraperURLConflicts closes with its accept button, disabled during a job
    const props = render(
      <ModalComponent show disabled onHide={hide} accept={{ onClick: hide }} />
    );
    pressEscape(props);
    expect(hide).not.toHaveBeenCalled();
  });

  it("still cancels when only the accept button is disabled", () => {
    const props = render(
      <ModalComponent
        show
        disabled
        cancel={{ onClick: cancel }}
        accept={{ onClick: accept }}
      />
    );
    pressEscape(props);
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it("does nothing when the dialog has no cancel action", () => {
    const props = render(<ModalComponent show accept={{ onClick: accept }} />);
    pressEscape(props);
    expect(accept).not.toHaveBeenCalled();
  });

  it("closes an open select menu first, not the dialog", () => {
    const props = render(
      <ModalComponent show onHide={hide} cancel={{ onClick: cancel }} />
    );
    pressEscape(props, { popup: "open" }, claimsEscape);
    expect(cancel).not.toHaveBeenCalled();
    expect(hide).not.toHaveBeenCalled();

    // the menu is closed now, so the next Esc closes the dialog
    pressEscape(props, { popup: "closed" }, claimsEscape);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(hide).not.toHaveBeenCalled();
  });

  it("leaves an open dropdown menu to close itself, whichever listener runs first", () => {
    const props = render(<ModalComponent show cancel={{ onClick: cancel }} />);
    // the dropdown's document listener runs after the dialog's here
    pressEscape(props, { inOpenDropdownMenu: true });
    expect(cancel).not.toHaveBeenCalled();
  });

  it("leaves Esc to anything else in the dialog that handles it", () => {
    // e.g. an open date picker
    const props = render(
      <ModalComponent show onHide={hide} cancel={{ onClick: cancel }} />
    );
    pressEscape(props, undefined, claimsEscape);
    expect(cancel).not.toHaveBeenCalled();
    expect(hide).not.toHaveBeenCalled();
  });
});

describe("Esc in dialogs not built on ModalComponent (#1113)", () => {
  beforeEach(() => vi.clearAllMocks());

  it("cancels a confirmation alert", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    const props = render(
      <AlertModal show text="Sure?" onCancel={onCancel} onConfirm={onConfirm} />
    );
    pressEscape(props);
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("cancels the save and load filter dialogs, but not while saving", () => {
    const onClose = vi.fn();
    pressEscape(
      render(<LoadFilterDialog mode={FilterMode.Scenes} onClose={onClose} />)
    );
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenLastCalledWith();

    pressEscape(
      render(<SaveFilterDialog mode={FilterMode.Scenes} onClose={onClose} />)
    );
    expect(onClose).toHaveBeenCalledTimes(2);
    expect(onClose).toHaveBeenLastCalledWith();

    pressEscape(
      render(
        <SaveFilterDialog mode={FilterMode.Scenes} onClose={onClose} isSaving />
      )
    );
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

describe("Esc during IME input (#1113)", () => {
  beforeEach(() => vi.clearAllMocks());

  it("leaves the dialog open", () => {
    const cancel = vi.fn();
    const hide = vi.fn();
    pressEscape(
      render(<ModalComponent show cancel={{ onClick: cancel }} />),
      undefined,
      undefined,
      true
    );
    pressEscape(
      render(<ModalComponent show onHide={hide} />),
      undefined,
      undefined,
      true
    );
    expect(cancel).not.toHaveBeenCalled();
    expect(hide).not.toHaveBeenCalled();
  });

  it("counts both ways browsers report composition", () => {
    const key = (init: Partial<KeyboardEvent>) =>
      ({ isComposing: false, keyCode: 27, ...init }) as KeyboardEvent;
    expect(isComposing(key({ isComposing: true }))).toBe(true);
    expect(isComposing(key({ keyCode: 229 }))).toBe(true);
    expect(isComposing(key({}))).toBe(false);
  });
});

describe("Esc in Close-only dialogs (#1113)", () => {
  it("closes the release notes", () => {
    const onClose = vi.fn();
    pressEscape(
      render(
        <ReleaseNotesDialog
          notes={[{ date: 1, version: "v1", content: "" }]}
          onClose={onClose}
        />
      )
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("cancels the tagger field selector without saving", () => {
    const onSelect = vi.fn();
    const onCancel = vi.fn();
    pressEscape(
      render(
        <FieldSelector
          show
          fields={["title", "date"]}
          excludedFields={["date"]}
          onSelect={onSelect}
          onCancel={onCancel}
        />
      )
    );
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onSelect).not.toHaveBeenCalled();
  });
});

describe("Esc with the filter sidebar open (#1113)", () => {
  it("hides the sidebar when no dialog is open", () => {
    const setShowSidebar = vi.fn();
    const e = { preventDefault: vi.fn() };
    hideSidebarOnEscape(e, true, setShowSidebar, false);
    expect(setShowSidebar).toHaveBeenCalledWith(false);
    expect(e.preventDefault).toHaveBeenCalled();
  });

  it("leaves Esc to an open dialog", () => {
    const setShowSidebar = vi.fn();
    const e = { preventDefault: vi.fn() };
    hideSidebarOnEscape(e, true, setShowSidebar, true);
    expect(setShowSidebar).not.toHaveBeenCalled();
    expect(e.preventDefault).not.toHaveBeenCalled();
  });
});
