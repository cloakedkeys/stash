import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// There's no DOM in these tests, so React's hooks are stood in for and the
// component is called directly. This runs the trigger's own tap and hover
// handlers (#1972).
const state = vi.hoisted(() => ({
  show: false,
  setShow: vi.fn(),
  hoverNone: false,
}));

vi.mock("react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react")>();
  return {
    ...actual,
    useState: (initial: unknown) =>
      initial === false ? [state.show, state.setShow] : [null, vi.fn()],
    useCallback: <T>(fn: T) => fn,
    useEffect: () => {},
    useRef: () => ({ current: "id" }),
  };
});

import { HoverPopover } from "./HoverPopover";

interface ITrigger {
  onClick: () => void;
  onMouseEnter: () => void;
}

function trigger() {
  const tree = (HoverPopover as (p: object) => React.ReactElement)({
    content: "Performers",
    children: "1",
  }) as React.ReactElement<{ children: React.ReactElement<ITrigger>[] }>;
  return React.Children.toArray(
    tree.props.children
  )[0] as React.ReactElement<ITrigger>;
}

beforeEach(() => {
  state.show = false;
  state.setShow.mockClear();
  vi.useFakeTimers();
  vi.stubGlobal("window", {
    matchMedia: () => ({ matches: state.hoverNone }),
    setTimeout,
    clearTimeout,
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("HoverPopover on a phone (no hover)", () => {
  beforeEach(() => {
    state.hoverNone = true;
  });

  it("opens on a tap", () => {
    trigger().props.onClick();
    expect(state.setShow).toHaveBeenCalledWith(true);
  });

  it("closes on a second tap", () => {
    state.show = true;
    trigger().props.onClick();
    expect(state.setShow).toHaveBeenCalledWith(false);
  });

  it("ignores the mouse events a tap also sends", () => {
    trigger().props.onMouseEnter();
    vi.runAllTimers();
    expect(state.setShow).not.toHaveBeenCalled();
  });
});

describe("HoverPopover with a mouse", () => {
  beforeEach(() => {
    state.hoverNone = false;
  });

  it("opens on hover, as before", () => {
    trigger().props.onMouseEnter();
    vi.runAllTimers();
    expect(state.setShow).toHaveBeenCalledWith(true);
  });

  it("does nothing on a click", () => {
    trigger().props.onClick();
    expect(state.setShow).not.toHaveBeenCalled();
  });
});
