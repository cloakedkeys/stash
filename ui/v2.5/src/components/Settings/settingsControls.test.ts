import { readFileSync } from "node:fs";
import { expect, it } from "vitest";

const styles = readFileSync(new URL("./styles.scss", import.meta.url), "utf8");

it("lays out save feedback and Close in one row instead of overlapping (#1959)", () => {
  const controls = styles.match(/\.settings-controls \{([^}]+)/)?.[1];
  expect(controls).toContain("display: flex;");
  expect(controls).toContain("gap: 1rem;");
  for (const selector of [".loading-indicator", ".close.settings-close"]) {
    const rule = styles.slice(styles.indexOf(`${selector} {`));
    const declarations = rule.slice(0, rule.indexOf("}"));
    expect(declarations).not.toMatch(/position:\s*fixed/);
    expect(declarations).not.toMatch(/(?:top|right|bottom):/);
  }
});
