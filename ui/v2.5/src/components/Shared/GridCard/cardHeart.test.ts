import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rules that put the
// favourite heart at the bottom-left of the picture, away from the rating
// ribbon in the top-left corner (#1978).
const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");
const cards = styles.slice(styles.indexOf(".scene-card,"));
const heart = cards.slice(
  cards.indexOf("button.btn.favorite-button {"),
  cards.indexOf("}", cards.indexOf("button.btn.favorite-button {"))
);

describe("favourite heart on cards", () => {
  it("is placed against the picture, not the whole card", () => {
    expect(cards).toMatch(/\.thumbnail-section\s*\{\s*position: relative;/);
  });

  it("sits at the bottom-left, not the top", () => {
    expect(heart).toContain("bottom: 0.5rem;");
    expect(heart).toContain("left: 0.5rem;");
    expect(heart).not.toContain("top:");
  });
});
