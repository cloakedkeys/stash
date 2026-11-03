import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The panel's markup needs a player, so this reads the order the sections are
// written in: on a phone the Actions buttons were below every slider (#1979).
const source = readFileSync(
  new URL("./SceneVideoFilterPanel.tsx", import.meta.url),
  "utf8"
);
const layout = source.slice(source.lastIndexOf("return ("));

describe("SceneVideoFilterPanel layout", () => {
  it("shows the Actions buttons above the filters", () => {
    const actions = layout.indexOf('id="actions_name"');
    const buttons = layout.indexOf("{renderResetButton()}");
    const filters = layout.indexOf('id="effect_filters.name"');
    expect(actions).toBeGreaterThan(-1);
    expect(filters).toBeGreaterThan(-1);
    expect(actions).toBeLessThan(filters);
    expect(buttons).toBeLessThan(filters);
  });
});
