import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// On a new group, studio or tag that hasn't been saved, going to an existing
// one in the same section skipped the unsaved-changes question, because the
// panels made an exception for the move that follows creating one (#1899).
// They now use the same question as every other move, like performers do.
// navigation.test.ts checks the question; the browser check is in the PR.
const panels = {
  groups: ["../components/Groups/GroupDetails/GroupEditPanel.tsx", "group"],
  studios: [
    "../components/Studios/StudioDetails/StudioEditPanel.tsx",
    "studio",
  ],
  tags: ["../components/Tags/TagDetails/TagEditPanel.tsx", "tag"],
};

describe.each(Object.entries(panels))("%s edit panel", (section, [
  path,
  obj,
]) => {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");

  it("asks on every move away with unsaved edits", () => {
    expect(source).toMatch(
      new RegExp(
        `message=\\{handleUnsavedChanges\\(\\s*intl,\\s*"${section}",\\s*${obj}\\.id,\\s*pathname\\s*\\)\\}`
      )
    );
    expect(source).not.toContain("isRedirectAfterCreate");
  });

  // creating a new item moves to its page; the form, and its question, isn't
  // shown while the save runs
  it("hides the form while saving", () => {
    const loading = source.indexOf("if (isLoading) return <LoadingIndicator");
    expect(loading).toBeGreaterThan(-1);
    expect(source.indexOf("<Prompt")).toBeGreaterThan(loading);
  });
});
