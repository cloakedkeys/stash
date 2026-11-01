const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

const itemTypes = [
  ["Scene", "Scenes", "scenes"],
  ["Image", "Images", "images"],
  ["Gallery", "Galleries", "galleries"],
  ["Group", "Groups", "groups"],
  ["Performer", "Performers", "performers"],
];

// Execute the actual URL handler with its query, toast, and state dependencies
// replaced. No browser or server is needed to exercise an empty scrape result.
function setup(item, directory, query, isNew = false) {
  const source = ts.createSourceFile(
    `${item}EditPanel.tsx`,
    fs.readFileSync(
      path.join(
        __dirname,
        `../src/components/${directory}/${item}Details/${item}EditPanel.tsx`
      ),
      "utf8"
    ),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX
  );
  const name = `onScrape${item}URL`;
  let handler;
  function visit(node) {
    if (ts.isFunctionDeclaration(node) && node.name?.text === name) {
      handler = node;
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.ok(handler, `${name} exists`);

  const calls = {
    queries: [],
    loading: [],
    toasts: [],
    errors: [],
    results: [],
  };
  const compiled = ts.transpileModule(handler.getText(source), {
    compilerOptions: { target: ts.ScriptTarget.ES2020 },
  });
  const run = vm.runInNewContext(`${compiled.outputText}\n${name}`, {
    isNew,
    [`queryScrape${item}URL`]: async (url) => {
      calls.queries.push(url);
      return query();
    },
    setIsLoading: (value) => calls.loading.push(value),
    Toast: {
      success: (message) => calls.toasts.push(message),
      error: (error) => calls.errors.push(error),
    },
    [`setScraped${item}`]: (value) => calls.results.push(["dialog", value]),
    [`update${item}EditStateFromScraper`]: (value) =>
      calls.results.push(["update", value]),
  });
  return { run, calls };
}

for (const [item, directory, plural] of itemTypes) {
  test(`${item} URL scrape reports no result and clears loading`, async () => {
    for (const isNew of [false, true]) {
      for (const result of [
        { data: { [`scrape${item}URL`]: null } },
        { data: {} },
        {},
      ]) {
        const { run, calls } = setup(item, directory, () => result, isNew);
        await run("https://example.com/item");
        assert.deepEqual(calls.queries, ["https://example.com/item"]);
        assert.deepEqual(calls.loading, [true, false]);
        assert.deepEqual(calls.toasts, [`No ${plural} found`]);
        assert.deepEqual(calls.errors, []);
        assert.deepEqual(calls.results, []);
      }
    }
  });

  test(`${item} URL scrape preserves successful results`, async () => {
    for (const isNew of [false, true]) {
      const scraped = { title: "Scraped title", name: "Scraped name" };
      const { run, calls } = setup(
        item,
        directory,
        () => ({ data: { [`scrape${item}URL`]: scraped } }),
        isNew
      );
      await run("https://example.com/item");
      const action =
        isNew && ["Group", "Performer"].includes(item) ? "update" : "dialog";
      assert.deepEqual(calls.results, [[action, scraped]]);
      assert.deepEqual(calls.toasts, []);
      assert.deepEqual(calls.errors, []);
      assert.deepEqual(calls.loading, [true, false]);
    }
  });

  test(`${item} URL scrape reports errors and clears loading`, async () => {
    const error = new Error("Scraper failed");
    const { run, calls } = setup(item, directory, () => {
      throw error;
    });
    await run("https://example.com/item");
    assert.deepEqual(calls.errors, [error]);
    assert.deepEqual(calls.toasts, []);
    assert.deepEqual(calls.results, []);
    assert.deepEqual(calls.loading, [true, false]);
  });

  test(`${item} URL scrape ignores an empty URL`, async () => {
    const { run, calls } = setup(item, directory, () => assert.fail());
    await run("");
    assert.deepEqual(calls, {
      queries: [],
      loading: [],
      toasts: [],
      errors: [],
      results: [],
    });
  });
}
