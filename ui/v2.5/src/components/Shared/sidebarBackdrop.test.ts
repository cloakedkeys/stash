import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule that dims the
// page behind an open filter panel on a phone and catches taps on it (#1961).
const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");
const sidebar = readFileSync(new URL("./Sidebar.tsx", import.meta.url), "utf8");

describe("filter panel on a phone", () => {
  const start = styles.indexOf(
    "&:not(.hide-sidebar) > .sidebar-pane-content::after {"
  );
  const rule = styles.slice(start, styles.indexOf("}", start));

  it("covers the page behind it, just under the panel", () => {
    expect(start).toBeGreaterThan(-1);
    expect(rule).toContain('content: "";');
    expect(rule).toContain("position: fixed;");
    expect(rule).toContain("inset: 0;");
    expect(rule).toContain("z-index: 99;");
    // the panel itself
    expect(styles).toMatch(/\.sidebar \{[^}]*z-index: 100;/);
  });

  it("only where the panel lies over the page", () => {
    // the nearest media rule above it is the sm-and-below one
    const before = styles.slice(0, start);
    expect(before.slice(before.lastIndexOf("@include"))).toMatch(
      /^@include media-breakpoint-down\(sm\) \{\s*$/
    );
    // sm and below is below 768px, where Sidebar.tsx closes it on a tap outside
    expect(sidebar).toContain('"only screen and (max-width: 767px)"');
  });
});
