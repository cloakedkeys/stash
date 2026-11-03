import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";
import { scrollToTopic, stackedQuery } from "./scrollToTopic";

const manual = readFileSync(new URL("./Manual.tsx", import.meta.url), "utf8");

describe("choosing a Help topic (#1969)", () => {
  it("scrolls to the topic's text when the list is above it", () => {
    const content = { scrollIntoView: vi.fn() };
    scrollToTopic(content, (q) => ({ matches: q === stackedQuery }));
    expect(content.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  it("doesn't scroll when the list and text are side by side", () => {
    const content = { scrollIntoView: vi.fn() };
    scrollToTopic(content, () => ({ matches: false }));
    expect(content.scrollIntoView).not.toHaveBeenCalled();
  });

  it("is used when a topic is chosen and when a link opens another topic", () => {
    expect(manual).toContain("scrollToTopic(contentRef.current");
    expect(manual).toContain("onSelect={(k) => k && showTopic(k)}");
    expect(manual).toContain("showTopic(newKey);");
    expect(manual).toContain("ref={contentRef}");
  });
});
