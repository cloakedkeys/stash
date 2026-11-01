import { describe, expect, it } from "vitest";
import { createIntl } from "react-intl";
import { handleUnsavedChanges } from "./navigation";

const intl = createIntl({
  locale: "en",
  messages: { "dialogs.unsaved_changes": "Unsaved changes" },
});

describe.each([
  ["performers", "/performers/1/scenes"],
  ["performers", "/performers/1/images/"],
  ["groups", "/groups/1/scenes"],
  ["groups", "/groups/1/scenes/"],
  ["galleries", "/galleries/1/add"],
  ["studios", "/studios/1"],
  ["tags", "/tags/1"],
])("unsaved %s editor at %s", (basepath, currentPath) => {
  it("allows viewer history on the same pathname", () => {
    const prompt = handleUnsavedChanges(intl, basepath, "1", currentPath);
    const destination = {
      pathname: currentPath,
      search: "?sortby=name",
      hash: "#image",
    };
    expect(prompt(destination)).toBe(true);
  });

  it("warns when leaving the current pathname", () => {
    const prompt = handleUnsavedChanges(intl, basepath, "1", currentPath);
    for (const pathname of [
      "/" + basepath + "/2",
      "/" + basepath + "/2/scenes",
      "/" + basepath + "/1/another-tab",
      "/" + basepath,
      "/scenes",
    ]) {
      expect(prompt({ pathname })).toBe("Unsaved changes");
    }
  });
});

it("keeps the gallery Add entity-root exception from #2291", () => {
  const prompt = handleUnsavedChanges(
    intl,
    "galleries",
    "1",
    "/galleries/1/add"
  );
  expect(prompt({ pathname: "/galleries/1" })).toBe(true);
  expect(prompt({ pathname: "/galleries/2" })).toBe("Unsaved changes");
});

it("allows the same pathname on a new performer form but warns when leaving", () => {
  const prompt = handleUnsavedChanges(
    intl,
    "performers",
    undefined,
    "/performers/new"
  );
  expect(prompt({ pathname: "/performers/new" })).toBe(true);
  for (const pathname of [
    "/performers/undefined",
    "/performers/1",
    "/performers",
  ]) {
    expect(prompt({ pathname })).toBe("Unsaved changes");
  }
});
