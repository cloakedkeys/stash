import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const source = readFileSync(
  new URL("./SceneCreate.tsx", import.meta.url),
  "utf8"
);

// How the split page uses copySceneMarkers (#1368). The copying itself is in
// copySceneMarkers.test.ts; the browser check is in the PR.
describe("split page marker copy", () => {
  it("offers the box only when the original scene has markers", () => {
    expect(source).toMatch(
      /const markers = query\.has\("from_scene_id"\)\s*\?\s*\(data\?\.findScene\?\.scene_markers \?\? \[\]\)/
    );
    expect(source).toMatch(
      /\{markers\.length > 0 && \(\s*<Form\.Group controlId="copy-scene-markers">/
    );
    expect(source).toMatch(/useState\(false\)/);
  });

  it("copies to the new scene, when ticked, after it is created", () => {
    const created = source.indexOf("if (result.data?.sceneCreate?.id) {");
    const copy = source.indexOf("await copySceneMarkers(");
    expect(created).toBeGreaterThan(-1);
    expect(copy).toBeGreaterThan(created);
    expect(source.slice(copy)).toMatch(
      /^await copySceneMarkers\(\s*markers,\s*result\.data\.sceneCreate\.id,[\s\S]*?createMarker\(\{ variables \}\)[\s\S]*?copyMarkers\s*\);/
    );
  });

  it("unticks the box after saving, so 'save and new' starts unticked", () => {
    const copy = source.indexOf("await copySceneMarkers(");
    const reset = source.indexOf("setCopyMarkers(false);");
    expect(reset).toBeGreaterThan(copy);
    expect(source.slice(copy, reset)).not.toContain("history.push");
  });
});
