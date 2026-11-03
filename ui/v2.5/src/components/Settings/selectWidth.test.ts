import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rules that make a
// drop-down wide enough for its longest choice without covering it with the
// arrow, and without sticking out of the settings card on a phone (#1960).
const settings = readFileSync(
  new URL("./styles.scss", import.meta.url),
  "utf8"
);
const index = readFileSync(
  new URL("../../index.scss", import.meta.url),
  "utf8"
);

describe("drop-downs in Settings", () => {
  it("are as wide as their longest choice", () => {
    expect(settings).toMatch(/select\.form-control \{\s*width: auto;/);
  });

  it("keep their side of the row from shrinking", () => {
    expect(settings).toMatch(
      /> div:last-child:has\(select\.form-control\) \{\s*flex-shrink: 0;/
    );
  });

  it("leave room for the arrow", () => {
    const start = index.indexOf("\nselect {");
    const select = index.slice(start, index.indexOf("\n}", start));
    expect(select).toContain("background-size: 1rem 2rem;");
    expect(select).toMatch(/&\.form-control \{\s*padding-right: 1\.5rem;/);
  });
});
