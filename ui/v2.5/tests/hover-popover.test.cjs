const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

function render(props = {}, component = "HoverPopover") {
  let overlayProps;
  const module = { exports: {} };
  const source = fs.readFileSync(
    path.join(__dirname, "../src/components/Shared/HoverPopover.tsx"),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      jsx: ts.JsxEmit.ReactJSX,
      esModuleInterop: true,
    },
  });
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    document: { body: {} },
    require: (name) => {
      if (name === "react-bootstrap") {
        return {
          Overlay: (props) => {
            overlayProps = props;
            return null;
          },
          Popover: "div",
        };
      }
      if (name === "src/patch")
        return { PatchComponent: (_name, component) => component };
      if (name === "./Icon") return { Icon: () => null };
      return require(name);
    },
  });
  renderToStaticMarkup(
    React.createElement(module.exports[component], {
      content: "Details",
      target: { current: {} },
      ...props,
    })
  );
  return overlayProps;
}

function assertViewportPositioning(props) {
  assert.equal(props.flip, true);
  for (const name of ["flip", "preventOverflow"]) {
    const modifier = props.popperConfig.modifiers.find((m) => m.name === name);
    assert.equal(modifier.enabled, true);
    assert.equal(modifier.options.rootBoundary, "viewport");
    assert.equal(modifier.options.boundary.length, 0);
  }
}

test("hover popovers flip and prevent overflow against the viewport", () => {
  const props = render();
  assertViewportPositioning(props);
  assert.equal(props.placement, "top");
});

test("hover popovers preserve caller placement and Popper configuration", () => {
  const offset = { name: "offset", options: { offset: [0, 6] } };
  const popperConfig = { strategy: "fixed", modifiers: [offset] };
  const props = render({ placement: "bottom-end", popperConfig });
  assertViewportPositioning(props);
  assert.equal(props.placement, "bottom-end");
  assert.equal(props.popperConfig.strategy, "fixed");
  assert.ok(props.popperConfig.modifiers.includes(offset));
  assert.equal(popperConfig.modifiers.length, 1);
});

test("warning hover popovers inherit viewport positioning", () => {
  assertViewportPositioning(render({}, "WarningHoverPopover"));
});
