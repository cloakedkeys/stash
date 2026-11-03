import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import enGB from "../../../locales/en-GB.json";

const tasks = readFileSync(
  new URL("./DataManagementTasks.tsx", import.meta.url),
  "utf8"
);
const styles = readFileSync(new URL("../styles.scss", import.meta.url), "utf8");

// Settings → Tasks → Migrations and Interface → Editing on a phone (#1950):
// long button names, options indented past their heading, and a four-line
// description.
describe("migration tasks", () => {
  const migrations = tasks.slice(
    tasks.indexOf('<SettingSection headingID="config.tasks.migrations">')
  );

  it("uses short button names", () => {
    expect(migrations).toContain('<FormattedMessage id="actions.rename" />');
    expect(migrations).toContain(
      '<FormattedMessage id="actions.migrate_screenshots" />'
    );
    expect(enGB.actions.rename).toBe("Rename");
    expect(enGB.actions.migrate_screenshots).toBe("Migrate Screenshots");
  });

  it("lines up each migration's options with its heading", () => {
    expect(
      migrations.match(/className="setting-group migration-group"/g)
    ).toHaveLength(2);
    expect(styles).toMatch(
      /&\.migration-group > \.setting:not\(:first-child\)\s*\{\s*margin-left: 0;\s*margin-right: 0;\s*padding-left: 15px;\s*padding-right: 15px;/
    );
  });

  it("keeps other task groups as they were", () => {
    const before = tasks.slice(
      0,
      tasks.indexOf('<SettingSection headingID="config.tasks.migrations">')
    );
    expect(before).not.toContain("migration-group");
  });
});

describe("auto-save on selection", () => {
  it("has a short description", () => {
    const { description } = enGB.config.ui.editing.auto_save_confirmed_fields;
    expect(description.length).toBeLessThanOrEqual(90);
  });
});
