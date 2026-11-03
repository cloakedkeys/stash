import { readFileSync } from "node:fs";
import { join } from "node:path";
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

function fakeWindow(doc: Document, touch: { hoverNone: boolean }) {
  return {
    document: doc,
    matchMedia: () => ({ matches: touch.hoverNone }),
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
    installTouchTooltipFade(fakeWindow(doc, { hoverNone: true }));
    tap();
    tap();
    expect(animations).toEqual(["none", "", "none", ""]);
  });

  it("does nothing where the mouse can hover", () => {
    const { doc, animations, tap } = fakePage();
    installTouchTooltipFade(fakeWindow(doc, { hoverNone: false }));
    tap();
    expect(animations).toEqual([]);
  });

  it("follows a switch from mouse to touch without a reload", () => {
    const { doc, animations, tap } = fakePage();
    const touch = { hoverNone: false };
    installTouchTooltipFade(fakeWindow(doc, touch));
    tap();
    touch.hoverNone = true;
    tap();
    expect(animations).toEqual(["none", ""]);
  });

  // The fade itself is CSS. A browser check is in the pull request; this
  // keeps the rule from being lost.
  it("fades shown hints out on touch screens only", () => {
    const scss = readFileSync(join(__dirname, "..", "index.scss"), "utf8");
    const media = scss.match(
      /@media \(hover: none\) \{\s*\.tooltip\.show \{\s*animation: (\S+) [^;]*\s(\d+(?:\.\d+)?)s forwards;/
    );
    expect(media).not.toBeNull();
    const [, name, delay] = media!;
    expect(Number(delay)).toBeGreaterThanOrEqual(2);
    const frames = scss.match(
      new RegExp(`@keyframes ${name} \\{\\s*to \\{([^}]*)\\}`)
    );
    expect(frames?.[1]).toMatch(/opacity: 0;/);
    expect(frames?.[1]).toMatch(/visibility: hidden;/);
  });
});
