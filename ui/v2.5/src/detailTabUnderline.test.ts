import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(new URL("./index.scss", import.meta.url), "utf8");

// The selected tab's underline used to end 4px above the tab bar's line, and
// only the selected or hovered tab had a border, so picking a tab made its row
// taller and moved the wrapped tabs below it (#2009). The browser check is
// described in the PR.
describe("detail page tab underline", () => {
  const start = styles.indexOf(".detail-body {");
  const nav = /\n {2}nav \{([\s\S]*?)\n {2}\}/.exec(styles.slice(start))?.[1];

  it("leaves no space under the tabs", () => {
    expect(start).toBeGreaterThan(-1);
    expect(nav).toMatch(/border-bottom: solid 2px #192127;/);
    expect(nav).toMatch(/padding: 5px 0 0;/);
  });

  it("gives every tab room for the underline, over the bar's line", () => {
    expect(nav).toMatch(
      /\.nav-link\s*\{\s*border-bottom: 2px solid transparent;\s*margin-bottom: -2px;\s*\}/
    );
  });
});
