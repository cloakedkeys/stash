import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

// The overlays can't be positioned without a browser, so this checks that each
// one that opened off screen stays on the shared rules.
it.each([
  "./HoverPopover.tsx",
  "./ImageInput.tsx",
  "../Tagger/PerformerCollisionWarningButton.tsx",
  "../../hooks/Lightbox/Lightbox.tsx",
])("%s keeps its popover inside the window", (file) => {
  const source = readFileSync(resolve(__dirname, file), "utf8");
  expect(source).toMatch(/\bflip\b/);
  expect(source).toMatch(/modifiers: (\[\s*)?(\.\.\.)?viewportModifiers/);
});
