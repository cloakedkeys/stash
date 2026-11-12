const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");

// Execute the real operations renderer, with surrounding page services stubbed.
test("scene operations opens an unsaved group form for this scene", () => {
  const source = fs.readFileSync(
    path.join(__dirname, "../src/components/Scenes/SceneDetails/Scene.tsx"),
    "utf8"
  );
  const ast = ts.createSourceFile(
    "Scene.tsx",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX
  );
  let renderer;
  function visit(node) {
    if (
      ts.isVariableDeclaration(node) &&
      node.name.getText(ast) === "renderOperations"
    )
      renderer = node.initializer.getText(ast);
    ts.forEachChild(node, visit);
  }
  visit(ast);
  assert.ok(renderer);
  const js = ts.transpileModule(`const render = ${renderer}; render();`, {
    compilerOptions: { jsx: ts.JsxEmit.React, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const navigations = [];
  const menu = vm.runInNewContext(js, {
    React,
    Dropdown: Object.assign(() => null, {
      Toggle: "toggle",
      Menu: "menu",
      Item: "item",
    }),
    Icon: "icon",
    FormattedMessage: "message",
    faEllipsisV: {},
    intl: { formatMessage: ({ id }) => id },
    canAdminister: false,
    boxes: [],
    scene: { id: "4", files: [] },
    history: { push: (url) => navigations.push(url) },
  });
  function find(element) {
    if (!React.isValidElement(element)) return undefined;
    if (element.props.children?.props?.id === "actions.make_group_from_scene")
      return element;
    for (const child of React.Children.toArray(element.props.children)) {
      const found = find(child);
      if (found) return found;
    }
  }
  const action = find(menu);
  assert.ok(action, "make-group action must be in the scene operations menu");
  const messages = JSON.parse(
    fs.readFileSync(path.join(__dirname, "../src/locales/en-GB.json"), "utf8")
  );
  assert.equal(
    messages.actions.make_group_from_scene,
    "Make a group from this scene\u2026"
  );
  assert.deepEqual(navigations, []);
  action.props.onClick();
  assert.deepEqual(navigations, ["/groups/new?from_scene_id=4"]);
});
