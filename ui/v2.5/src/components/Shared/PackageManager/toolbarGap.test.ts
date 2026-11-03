import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule; the browser
// check is described in the PR (#1948).
const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");
const available = styles.slice(styles.indexOf(".available-packages {"));

describe("Available packages toolbar on a phone", () => {
  it("hides the spacer, so the wrapped buttons have even gaps", () => {
    expect(available).toMatch(
      /@include media-breakpoint-down\(xs\) \{\s*> \.flex-grow-1 \{\s*display: none;/
    );
  });
});
