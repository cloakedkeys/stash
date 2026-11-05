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

// An element a key is pressed on or clicked: a react-select input or dropdown
// toggle (a popup trigger, open or not), something inside an open dropdown
// menu, a control that changes a value (a button, a select option), something
// in the dialog's header or footer, or a plain element such as a text box.
type Target =
  | { popup: "open" | "closed" }
  | { inOpenDropdownMenu: true }
  | { valueControl: true }
  | { inFooter: true }
  | undefined;

function element(target: Target): Partial<Element> {
  const attributes: Record<string, string> = {};
  if (target && "popup" in target) {
    attributes["aria-haspopup"] = "true";
    attributes["aria-expanded"] = target.popup === "open" ? "true" : "false";
  }
  const matches = (selector: string) =>
    (!!target &&
      "inOpenDropdownMenu" in target &&
      selector.includes(".dropdown-menu.show")) ||
    (!!target && "valueControl" in target && selector.includes("button")) ||
    (!!target && "inFooter" in target && selector.includes(".modal-footer"));
  return {
    getAttribute: (name: string) => attributes[name] ?? null,
    closest: ((selector: string) =>
      matches(selector) ? ({} as Element) : null) as Element["closest"],
  };
}

type Handler = ((e: Partial<React.SyntheticEvent>) => void) | undefined;
const handler = (props: ModalProps, name: string) =>
  (props as Record<string, Handler>)[name];

// Types into a text box in the dialog.
function type(props: ModalProps) {
  const e = { type: "input", target: element(undefined) as EventTarget };
  handler(props, "onInputCapture")?.(e);
  handler(props, "onChangeCapture")?.({ ...e, type: "change" });
}

// Presses the mouse on, and clicks, an element in the dialog.
function click(props: ModalProps, target: Target) {
  const el = element(target) as EventTarget;
  handler(props, "onMouseDownCapture")?.({ type: "mousedown", target: el });
  handler(props, "onClickCapture")?.({ type: "click", target: el });
}

// Presses a key other than Esc in the dialog.
function press(props: ModalProps, key: string, target: Target) {
  props.onKeyDownCapture?.({
    type: "keydown",
    key,
    target: element(target) as EventTarget,
  } as Partial<React.KeyboardEvent>);
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

describe("Esc once something in the dialog has changed (#1113)", () => {
  const cancel = vi.fn();
  const hide = vi.fn();
  const dialog = () =>
    render(<ModalComponent show cancel={{ onClick: cancel }} />);

  beforeEach(() => vi.clearAllMocks());

  it("does nothing after typing; Cancel is still there", () => {
    const props = dialog();
    type(props);
    pressEscape(props);
    expect(cancel).not.toHaveBeenCalled();
  });

  it("does nothing after a click that may change a value", () => {
    // a rating star, add/remove/set toggle, select option, scrape choice...
    const props = dialog();
    click(props, { valueControl: true });
    pressEscape(props);
    expect(cancel).not.toHaveBeenCalled();
  });

  it("does nothing after picking or removing a select value by keyboard", () => {
    for (const [key, popup] of [
      ["Enter", "open"],
      ["Tab", "open"],
      ["Backspace", "closed"],
      ["Delete", "closed"],
    ] as const) {
      const props = dialog();
      press(props, key, { popup });
      pressEscape(props);
    }
    expect(cancel).not.toHaveBeenCalled();
  });

  it("still cancels after clicks and keys that change nothing", () => {
    const props = dialog();
    click(props, undefined); // e.g. into a text box, or on a label
    click(props, { inFooter: true });
    press(props, "Tab", { popup: "closed" });
    press(props, "Enter", undefined);
    pressEscape(props);
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it("cancels again once the dialog is shown afresh", () => {
    const props = dialog();
    type(props);
    handler(props, "onShow")?.({});
    pressEscape(props);
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it("keeps cancelling in search and picker dialogs", () => {
    const props = render(
      <ModalComponent show escapeAfterChanges onHide={hide} />
    );
    type(props);
    click(props, { valueControl: true });
    pressEscape(props);
    expect(hide).toHaveBeenCalledTimes(1);
  });

  it("applies to the save filter dialog, not the load filter search", () => {
    const onClose = vi.fn();
    const save = render(
      <SaveFilterDialog mode={FilterMode.Scenes} onClose={onClose} />
    );
    type(save);
    pressEscape(save);
    expect(onClose).not.toHaveBeenCalled();

    const load = render(
      <LoadFilterDialog mode={FilterMode.Scenes} onClose={onClose} />
    );
    type(load);
    pressEscape(load);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("protects a field selector with toggled fields", () => {
    const onCancel = vi.fn();
    const props = render(
      <FieldSelector
        show
        fields={["title"]}
        excludedFields={[]}
        onSelect={() => {}}
        onCancel={onCancel}
      />
    );
    click(props, { valueControl: true });
    pressEscape(props);
    expect(onCancel).not.toHaveBeenCalled();
  });
});
