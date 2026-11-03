import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { scrollToTopic, stackedQuery } from "./scrollToTopic";

const manual = readFileSync(new URL("./Manual.tsx", import.meta.url), "utf8");

// the window's body, scrolled 100px, with the topic's text 900px down the screen
function setup() {
  const body = { scrollTop: 100, getBoundingClientRect: () => ({ top: 50 }) };
  const content = {
    closest: (s: string) => (s === ".modal-body" ? body : null),
    getBoundingClientRect: () => ({ top: 900 }),
  };
  return { body, content };
}

describe("choosing a Help topic (#1969)", () => {
  it("scrolls the window's body to the topic's text when the list is above it", () => {
    const { body, content } = setup();
    scrollToTopic(content, (q) => ({ matches: q === stackedQuery }));
    expect(body.scrollTop).toBe(100 + 850);
  });

  it("doesn't scroll when the list and text are side by side", () => {
    const { body, content } = setup();
    scrollToTopic(content, () => ({ matches: false }));
    expect(body.scrollTop).toBe(100);
  });

  it("is used when a topic is chosen and when a link opens another topic", () => {
    expect(manual).toContain("scrollToTopic(contentRef.current");
    expect(manual).toContain("showTopic(newKey);");
    expect(manual).toContain("ref={contentRef}");
  });

  it("not when the arrow keys move through the list", () => {
    expect(manual).toMatch(
      /onSelect=\{\(k, e\) => k && showTopic\(k, e\?\.type !== "keydown"\)\}/
    );
  });
});
