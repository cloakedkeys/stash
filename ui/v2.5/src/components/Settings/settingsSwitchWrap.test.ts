import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

it("keeps grouped switches beside wrapping labels only on phones", () => {
  const styles = readFileSync(resolve(__dirname, "styles.scss"), "utf8");

  // Grouped settings normally wrap the whole control onto another line.
  // Boolean settings must instead let their label shrink beside the switch.
  expect(styles).toMatch(
    /\.setting-group\s*\{[\s\S]*?\.setting\s*\{[\s\S]*?@include media-breakpoint-down\(xs\)\s*\{\s*&\.boolean-setting\s*\{\s*flex-wrap:\s*nowrap;\s*\}\s*\}/
  );
});
