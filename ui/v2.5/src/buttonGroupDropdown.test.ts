import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule that lifts a
// pressed dropdown button above its neighbour in a button group (#1981).
const styles = readFileSync(new URL("./index.scss", import.meta.url), "utf8");

describe("dropdown in a button group", () => {
  it("is raised above its neighbours while pressed or open", () => {
    const rule =
      /\.btn-group > \.dropdown:focus-within,\s*\.btn-group > \.dropdown\.show\s*\{\s*z-index: 1;/;
    expect(styles).toMatch(rule);
  });
});
