const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

function renderPaths(configuredStashes, setupState = {}) {
  let displayedStashes;
  let submitted;
  let next;
  const source = fs.readFileSync(
    path.join(__dirname, "../src/components/Setup/Setup.tsx"),
    "utf8"
  );
  // Expose the step and its context only in this test, without changing the UI API.
  const compiled = ts.transpileModule(
    `${source}\nexport { SetPathsStep, SetupStateContext };`,
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2020,
        jsx: ts.JsxEmit.ReactJSX,
        esModuleInterop: true,
      },
    }
  );
  const module = { exports: {} };
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require: (name) => {
      if (name === "react" || name === "react/jsx-runtime")
        return require(name);
      if (name === "react-intl") {
        return { FormattedMessage: () => null };
      }
      if (name === "react-bootstrap") {
        return {
          ...require(name),
          Button: ({ children, onClick }) => {
            if (children.props.id === "actions.next_action") next = onClick;
            return null;
          },
        };
      }
      if (name === "../Settings/StashConfiguration") {
        return {
          __esModule: true,
          default: ({ stashes }) => {
            displayedStashes = stashes;
            return React.createElement(
              "div",
              null,
              stashes.map((stash) => stash.path).join(",")
            );
          },
        };
      }
      if (name === "../Shared/ExternalLink") {
        return { ExternalLink: () => null };
      }
      // Other imports belong to wizard steps and controls not rendered here.
      return {};
    },
  });
  const { SetPathsStep, SetupStateContext } = module.exports;
  const html = renderToStaticMarkup(
    React.createElement(
      SetupStateContext.Provider,
      {
        value: {
          setupState,
          configuration: {
            general: {
              stashes: configuredStashes,
              databasePath: "database",
              generatedPath: "generated",
              cachePath: "cache",
              blobsPath: "blobs",
            },
          },
        },
      },
      React.createElement(SetPathsStep, {
        goBack() {},
        next: (input) => {
          submitted = input;
        },
      })
    )
  );
  return {
    html,
    displayedStashes,
    submit: () => {
      next();
      return submitted;
    },
  };
}

test("setup displays and submits the library folder supplied by configuration", () => {
  for (const folder of ["/data", "C:\\media", "/storage/emulated/0/Movies"]) {
    const stashes = [
      {
        path: folder,
        excludeVideo: false,
        excludeAudio: true,
        excludeImage: false,
        includeStory: true,
      },
    ];
    const step = renderPaths(
      stashes.map((stash) => ({ __typename: "StashConfig", ...stash }))
    );
    assert.ok(step.html.includes(folder));
    assert.deepEqual(
      JSON.parse(JSON.stringify(step.displayedStashes)),
      stashes
    );
    assert.deepEqual(
      JSON.parse(JSON.stringify(step.submit().stashes)),
      stashes
    );
  }
});

test("returning to the paths step preserves edited folders and exclusions", () => {
  const edited = [
    {
      path: "/chosen",
      excludeVideo: true,
      excludeAudio: false,
      excludeImage: true,
    },
  ];
  const step = renderPaths([{ path: "/data" }], { stashes: edited });
  assert.deepEqual(step.displayedStashes, edited);
  assert.deepEqual(step.submit().stashes, edited);
});

test("returning to the paths step preserves an explicitly emptied folder list", () => {
  assert.deepEqual(
    renderPaths([{ path: "/data" }], { stashes: [] }).displayedStashes,
    []
  );
});

test("setup starts empty when configuration has no library folders", () => {
  assert.equal(renderPaths([]).displayedStashes.length, 0);
});
