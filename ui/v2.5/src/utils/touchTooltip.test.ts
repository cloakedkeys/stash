import { describe, expect, it } from "vitest";
import { installTouchTooltipFade, restartTooltipFades } from "./touchTooltip";

// the UI tests run without a browser, so a tiny stand-in page
function fakePage() {
  const animations: string[] = [];
  const hint = {
    style: {
      set animation(v: string) {
        animations.push(v);
      },
    },
    offsetWidth: 10,
  };
  let onClick: (() => void) | undefined;
  const doc = {
    querySelectorAll: (selector: string) =>
      selector === ".tooltip.show" ? [hint] : [],
    addEventListener: (type: string, fn: () => void) => {
      if (type === "click") onClick = fn;
    },
  } as unknown as Document;
  const tap = () => onClick?.();
  return { doc, animations, tap };
}

function fakeWindow(doc: Document, hoverNone: boolean) {
  return {
    document: doc,
    matchMedia: () => ({ matches: hoverNone }),
    requestAnimationFrame: (cb: FrameRequestCallback) => {
      cb(0);
      return 0;
    },
  } as unknown as Window;
}

describe("touch tooltip fade", () => {
  it("restarts the fade of the hints on screen", () => {
    const { doc, animations } = fakePage();
    restartTooltipFades(doc);
    expect(animations).toEqual(["none", ""]);
  });

  it("restarts on each tap on a touch screen", () => {
    const { doc, animations, tap } = fakePage();
    installTouchTooltipFade(fakeWindow(doc, true));
    tap();
    tap();
    expect(animations).toEqual(["none", "", "none", ""]);
  });

  it("does nothing where the mouse can hover", () => {
    const { doc, animations, tap } = fakePage();
    installTouchTooltipFade(fakeWindow(doc, false));
    tap();
    expect(animations).toEqual([]);
  });
});
