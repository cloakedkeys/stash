import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync(
  new URL("../../index.scss", import.meta.url),
  "utf8"
);
const app = readFileSync(new URL("../../App.tsx", import.meta.url), "utf8");
const overlay = styles.slice(styles.indexOf(".troubleshooting-mode-overlay {"));
const phone = overlay.slice(
  overlay.indexOf("@include media-breakpoint-down(xs) {")
);

// The notice must occupy space after every page, including when its text wraps
// on a phone. Only the red frame should remain over the viewport (#1952).
describe("troubleshooting notice on a phone", () => {
  it("renders after the page content", () => {
    expect(app.indexOf("<TroubleshootingModeOverlay />")).toBeGreaterThan(
      app.indexOf("<MainContainer>{renderContent()}</MainContainer>")
    );
  });

  it("keeps both the wrapper and notice in normal flow on narrow screens", () => {
    expect(phone).toMatch(/^[^{]*\{\s*border: 0;\s*position: static;/);
    expect(phone).toMatch(
      /\.troubleshooting-mode-alert\s*\{[^}]*display: flex;[^}]*position: static;/
    );
  });

  it("keeps the viewport frame fixed without blocking page controls", () => {
    expect(overlay).toContain("pointer-events: none;");
    expect(phone).toMatch(
      /&::before\s*\{[^}]*border: 5px solid \$danger;[^}]*inset: 0;[^}]*position: fixed;/
    );
  });
});
