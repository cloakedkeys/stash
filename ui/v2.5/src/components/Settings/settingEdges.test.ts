import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rules that line up a
// section's description with its box, and a group's fields with its heading
// (#1948). The browser check is described in the PR.
const settings = readFileSync(
  new URL("./styles.scss", import.meta.url),
  "utf8"
);
const block = (start: string) => {
  const i = settings.indexOf(start);
  return i < 0 ? "" : settings.slice(i);
};

describe("left edges in Settings", () => {
  it("a section's box isn't inset from its heading and description", () => {
    expect(block("\n.setting-section {")).toMatch(
      /> \.card \{\s*margin-left: 0;\s*margin-right: 0;/
    );
  });

  it("fields and buttons in a group line up with its heading", () => {
    expect(block("  .setting-group {")).toMatch(
      /> \.form-group,\s*> \.btn \{\s*margin-left: 15px;\s*margin-right: 15px;/
    );
  });
});
