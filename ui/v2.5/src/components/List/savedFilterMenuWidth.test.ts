import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");
const list = readFileSync(
  new URL("./SavedFilterList.tsx", import.meta.url),
  "utf8"
);

// On a phone the saved filter menu slid left from its button over the
// controls beside it (#1955). It now spans the screen under the toolbar row.
// The browser check is in the PR.
describe("saved filter menu on a phone", () => {
  const menu = styles.slice(
    styles.indexOf(".saved-filter-list-menu {"),
    styles.indexOf("\n.saved-filter-list {")
  );

  it("spans the screen less a margin on each side", () => {
    expect(menu).toMatch(
      /@include media-breakpoint-down\(xs\)\s*\{\s*width: calc\(100vw - 16px\);\s*\}/
    );
  });

  it("keeps its width on wider screens", () => {
    expect(menu).toMatch(/^\.saved-filter-list-menu\s*\{\s*width: 300px;/);
  });

  it("keeps that margin from the screen's edges", () => {
    expect(list).toMatch(
      /modifiers: \[\{ name: "preventOverflow", options: \{ padding: 8 \} \}\]/
    );
    expect(list).toContain("popperConfig={savedFilterPopperConfig}");
  });
});
