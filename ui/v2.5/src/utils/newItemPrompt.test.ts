import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// On a new group, studio or tag that hasn't been saved, going to an existing
// one in the same section skipped the unsaved-changes question, because it
// looked like the move that follows creating one (#1899). The question is now
// only skipped while a save is running. The browser check is in the PR.
const panels = {
  groups: "../components/Groups/GroupDetails/GroupEditPanel.tsx",
  studios: "../components/Studios/StudioDetails/StudioEditPanel.tsx",
  tags: "../components/Tags/TagDetails/TagEditPanel.tsx",
};

describe.each(Object.entries(panels))("%s edit panel", (section, path) => {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const onSave = source.slice(
    source.indexOf("async function onSave("),
    source.indexOf("async function onSaveAndNewClick(")
  );

  it("marks the time a save is running", () => {
    expect(source).toContain("const saving = useRef(false);");
    const start = onSave.indexOf("saving.current = true;");
    const submit = onSave.indexOf("await onSubmit(");
    const end = onSave.indexOf("saving.current = false;");
    expect(start).toBeGreaterThan(-1);
    expect(submit).toBeGreaterThan(start);
    // after the try/catch, so a failed save clears it too
    expect(end).toBeGreaterThan(onSave.indexOf("Toast.error(e);"));
  });

  it("skips the question only while a new item is being saved", () => {
    expect(source).toMatch(
      new RegExp(
        `isRedirectAfterCreate\\(\\s*isNew && saving\\.current,\\s*action,\\s*location\\.pathname,\\s*"${section}"\\s*\\)`
      )
    );
    expect(source).not.toMatch(/isRedirectAfterCreate\(\s*isNew,/);
  });
});
