import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

it("keeps the clear button beside its field in taller phone toolbars", () => {
  const styles = readFileSync(resolve(__dirname, "styles.scss"), "utf8");
  const group = styles.match(/\.clearable-input-group\s*\{([\s\S]*?)\n\}/)?.[1];
  const phoneRules = group?.match(
    /@include media-breakpoint-down\(xs\)\s*\{([^}]+)\}/
  );

  // The clear button spans the wrapper, so it must not stretch to the height
  // of taller toolbar buttons when the text field itself stays one line high.
  expect(phoneRules?.[1]).toMatch(/align-self:\s*flex-start;/);
  // Leave the alignment on wider screens unchanged.
  expect(group?.split("@include")[0]).not.toMatch(/align-self:/);
});
