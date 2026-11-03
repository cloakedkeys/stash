import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");

// Help opened on a topic scrolls to its text straight away, before the text
// has loaded. Without room below, the scroll stopped short and left the topic
// list on screen (#2005). The browser check is in the PR.
describe("Help on a phone", () => {
  const phone = styles.slice(styles.indexOf("@media (max-width: 992px)"));

  it("keeps a screen's worth of room for the topic's text", () => {
    expect(phone).toMatch(/\.manual-content\s*\{\s*min-height: 100vh;\s*\}/);
  });

  it("only on the stacked layout", () => {
    const desktop = styles.slice(
      0,
      styles.indexOf("@media (max-width: 992px)")
    );
    expect(desktop).not.toContain("min-height");
  });
});
