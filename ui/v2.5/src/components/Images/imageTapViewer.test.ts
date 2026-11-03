import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule; the browser
// check is described in the PR (#1965).
const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");
const card = styles.slice(styles.indexOf("\n.image-card {"));
const preview = card.slice(card.indexOf("&-preview {"));

describe("tapping an image card's picture on a touch screen", () => {
  it("opens the viewer from anywhere on the picture", () => {
    expect(preview).toMatch(
      /@media \(hover: none\), \(pointer: coarse\) \{\s*\.preview-button button\.btn \{[^}]*height: 100%;[^}]*width: 100%;/
    );
  });
});
