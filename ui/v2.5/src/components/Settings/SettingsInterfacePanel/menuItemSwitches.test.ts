import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";

// Both lists are read from their source files, since the components that hold
// them can't be loaded without a browser.
function listIn(file: string, field: string) {
  const source = readFileSync(resolve(__dirname, file), "utf8");
  const start = source.indexOf("const allMenuItems");
  const list = source.slice(start, source.indexOf("\n];", start));
  return Array.from(
    list.matchAll(new RegExp(`\\b${field}: "([^"]+)"`, "g")),
    (m) => m[1]
  ).sort();
}

// Saving the switches saves only the items they list, so an item on the menu with
// no switch disappears from the menu the first time they're saved.
it("has a switch for every item on the menu", () => {
  expect(listIn("./SettingsInterfacePanel.tsx", "id")).toEqual(
    listIn("../../MainNavbar.tsx", "name")
  );
});
