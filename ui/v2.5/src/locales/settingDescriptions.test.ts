import { describe, expect, it } from "vitest";
import messages from "./en-GB.json";

// Settings and Tasks descriptions should fit a phone screen in a sentence
// or two, while keeping their warnings (#1950).
const get = (path: string) =>
  path
    .split(".")
    .reduce<unknown>(
      (o, k) => (o as Record<string, unknown>)[k],
      messages
    ) as string;

describe("Settings and Tasks descriptions", () => {
  it.each([
    "config.tasks.optimise_database",
    "config.tasks.optimise_database_warning",
    "config.tasks.migrate_blobs.description",
    "config.tasks.migrate_scene_screenshots.description",
    "config.ui.preview_type.description",
    "config.tasks.anonymise_database",
    "config.scraping.url_conflict.description",
    "config.general.generated_file_naming_hash_desc",
    "config.ui.use_stash_hosted_funscript.description",
    "config.stashbox.description",
    "config.general.blobs_storage.description",
  ])("%s is short", (path) => {
    expect(get(path).length).toBeLessThanOrEqual(200);
  });

  it("keep their warnings", () => {
    const optimise = get("config.tasks.optimise_database_warning");
    expect(optimise).toMatch(/fails/);
    expect(optimise).toMatch(/free disk space/);
    expect(optimise).toMatch(/1\.5/);
    expect(get("config.tasks.anonymise_database")).toContain(
      "{filename_format}"
    );
    expect(get("config.general.blobs_storage.description")).toMatch(
      /Migrate blobs/
    );
  });
});
