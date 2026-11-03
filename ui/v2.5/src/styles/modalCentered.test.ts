import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule that opens
// pop-ups in the middle of a phone screen (#1946).
const theme = readFileSync(new URL("./_theme.scss", import.meta.url), "utf8");

describe("pop-ups on a phone", () => {
  it("are centred on the screen", () => {
    const start = theme.indexOf("@include media-breakpoint-down(xs) {");
    expect(start).toBeGreaterThan(-1);
    const block = theme.slice(start, theme.indexOf("\n}", start));
    expect(block).toMatch(/\.modal-dialog \{/);
    expect(block).toContain("align-items: center;");
    expect(block).toContain("display: flex;");
    expect(block).toContain(
      "min-height: calc(100% - #{$modal-dialog-margin * 2});"
    );
  });
});
