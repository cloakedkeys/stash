import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// A task row must wrap inside the queue box on a phone (#1966). There's no
// browser in these tests, so this checks the rules that make it wrap.
const styles = readFileSync(new URL("../styles.scss", import.meta.url), "utf8");
const queue = styles.slice(styles.indexOf(".job-table.card {"));

function rules(selector: string) {
  const start = queue.indexOf(`${selector} {`);
  expect(start, `${selector} styles`).toBeGreaterThan(-1);
  return queue.slice(start, queue.indexOf("}", start));
}

describe("task queue row", () => {
  it("lets the status shrink beside the stop button and break long paths", () => {
    const status = rules(".job-status");
    expect(status).toContain("flex: 1;");
    expect(status).toContain("min-width: 0;");
    expect(status).toContain("overflow-wrap: anywhere;");
    expect(status).not.toContain("width: 100%;");
  });

  it("puts the ETA on its own line when it doesn't fit", () => {
    expect(rules(".job-description")).toContain("flex-wrap: wrap;");
  });
});
