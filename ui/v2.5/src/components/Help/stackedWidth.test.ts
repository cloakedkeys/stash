import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { stackedQuery } from "./scrollToTopic";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");

// Help's phone styles, its topic scrolling and Bootstrap's stacked columns
// switch at the same width. At exactly 992px they disagreed: phone styles
// with the topic list beside the text (#2015).
describe("Help's stacked layout width", () => {
  it("uses Bootstrap's breakpoint below lg, which is 991.98px", () => {
    expect(styles).toContain("@include media-breakpoint-down(md)");
    expect(styles).not.toMatch(/max-width:\s*992px/);
    expect(stackedQuery).toBe("(max-width: 991.98px)");
  });
});
