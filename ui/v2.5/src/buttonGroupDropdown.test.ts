import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// There's no browser in these tests, so this checks the rule that lifts a
// pressed dropdown button above its neighbour in a button group (#1981).
const styles = readFileSync(new URL("./index.scss", import.meta.url), "utf8");

describe("dropdown in a button group", () => {
  const block = styles.slice(
    styles.indexOf(".btn-group > .dropdown > .btn {"),
    styles.indexOf("dl.details-list")
  );

  it("raises the pressed button above its neighbours", () => {
    expect(block).toContain("position: relative;");
    expect(block).toMatch(
      /&:focus,\s*&:active,\s*&\.active\s*\{\s*z-index: 1;/
    );
  });

  it("doesn't raise the whole dropdown, which would trap its open menu", () => {
    expect(styles).not.toMatch(
      /\.btn-group > \.dropdown(:focus-within|\.show)/
    );
  });
});
