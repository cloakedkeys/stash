import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");

// Like cardHeart.test.ts, check the layout rules without a browser; the
// browser check is described in the PR. The toolbar's controls used to wrap
// wherever they ran out of room, so tabs laid them out differently (#1976).
describe("performer tab toolbars on phones", () => {
  const start = styles.indexOf("// The rows stay independent");
  const mobile = styles.slice(start, styles.indexOf("\n}\n", start));

  it("applies to performer pages on small screens only", () => {
    expect(start).toBeGreaterThan(-1);
    expect(mobile).toMatch(
      /#performer-page\s*\{\s*@include media-breakpoint-down\(xs\)\s*\{\s*\.filtered-list-toolbar:not\(\.has-selection\)\s*\{/
    );
  });

  it("forces a line break after the filters and before the views", () => {
    expect(mobile).toMatch(
      /&::before,\s*&::after\s*\{\s*content: "";\s*flex-basis: 100%;\s*height: 0;/
    );
  });

  it.each([
    [".search-term-input", 1],
    [".search-term-input + .btn-group", 2],
    [".sort-by-select", 4],
    [".page-size-selector", 5],
    [".list-operations", 6],
    [".display-mode-button-group", 8],
  ])("puts %s in place %i", (selector, order) => {
    expect(mobile).toContain(`> ${selector} {\n        order: ${order};`);
  });

  it("breaks between the rows", () => {
    expect(mobile).toContain("&::before {\n        order: 3;");
    expect(mobile).toContain("&::after {\n        order: 7;");
  });

  it("doesn't hide any control", () => {
    expect(mobile).not.toContain("display: none");
  });
});
