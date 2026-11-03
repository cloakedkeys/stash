import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const read = (path: string) =>
  readFileSync(new URL(path, import.meta.url), "utf8");

// Where the two ways of setting a performer's picture from the library are
// offered (#1977). The browser check is in the PR.
describe("performer picture from the library", () => {
  it("offers a scene cover on a saved performer's edit page", () => {
    const panel = read("./Performers/PerformerDetails/PerformerEditPanel.tsx");
    expect(panel).toMatch(
      /extraActions=\{\s*!isNew\s*\?\s*\[\s*\{\s*icon: faImage,\s*labelId: "actions\.set_from_scene_cover"/
    );
    // the chosen cover goes into the form, like a picture from a web address
    expect(panel).toMatch(
      /<SceneCoverPicker\s+performerId=\{performer\.id\}\s+onSelect=\{onImageLoad\}/
    );
  });

  it("offers the current frame beside the scene's other cover actions", () => {
    const panel = read("./Scenes/SceneDetails/SceneEditPanel.tsx");
    expect(panel).toMatch(
      /!isNew &&\s*!!scene\.files\?\.length &&\s*performers\.length > 0\s*\?\s*\[\s*\{\s*icon: faUser,\s*labelId: "actions\.set_as_performer_image",\s*onClick: performerPicture\.capture,/
    );
    // it isn't limited to admins, unlike the server-side thumbnail actions
    const frameAction = panel.indexOf("performerPicture.capture");
    const adminGate = panel.indexOf("!isNew && isAdmin");
    expect(adminGate).toBeGreaterThan(-1);
    expect(panel.slice(adminGate, frameAction)).toContain(": []),");
    expect(panel).toContain("{performerPicture.dialog}");
    // the performers shown in the form, not the last saved ones, so a
    // performer swapped in without saving gets the picture
    expect(panel).toContain("usePerformerPictureFromFrame(performers)");
  });

  it("closes the performer chooser with its button", () => {
    const hook = read("./Scenes/SceneDetails/usePerformerPictureFromFrame.tsx");
    // the dialog's main button is the accept one; without a handler it did nothing
    expect(hook).toMatch(/accept=\{\{[\s\S]*?onClick: close,\s*\}\}/);
  });
});
