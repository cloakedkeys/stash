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

// On a phone the notice takes its own space at the top of every page instead
// of being pinned over the bottom; only the red frame stays over the screen
// (#1952).
describe("troubleshooting notice on a phone", () => {
  it("comes before the page content, so it's at the top of the page", () => {
    expect(app.indexOf("<TroubleshootingModeOverlay />")).toBeGreaterThan(-1);
    expect(app.indexOf("<TroubleshootingModeOverlay />")).toBeLessThan(
      app.indexOf("<MainContainer>{renderContent()}</MainContainer>")
    );
  });

  it("keeps both the wrapper and notice in normal flow on narrow screens", () => {
    expect(phone).toMatch(/^[^{]*\{\s*border: 0;[^{]*position: static;/);
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

  it("keeps the frame above the nav bar", () => {
    // see-through on the wrapper would keep the frame beneath other layers
    expect(phone).toMatch(/^[^{]*\{[^{]*opacity: 1;/);
    expect(phone).toMatch(
      /&::before\s*\{[^}]*opacity: 0\.75;[^}]*z-index: 1040;/
    );
  });
});
