import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");

// The carousel arrows are placed outside each row. On a phone the rows reach
// the screen's edges, so the arrows made the home page wider than the screen
// and the bottom menu bar covered the Customize button (#1956). The browser
// check is in the PR.
describe("home page on a phone", () => {
  const container = styles.slice(
    styles.indexOf(".recommendations-container {"),
    styles.indexOf(".recommendations-footer {")
  );

  it("keeps the carousel arrows inside the screen", () => {
    expect(container).toMatch(
      /@media \(max-width: 576px\) \{[^}]*overflow-x: clip;[^}]*\}/
    );
  });

  it("still places the arrows outside the rows", () => {
    expect(styles).toMatch(/\.slick-prev \{\s*left: -20px;/);
    expect(styles).toMatch(/\.slick-next \{\s*right: -25px;/);
  });
});
