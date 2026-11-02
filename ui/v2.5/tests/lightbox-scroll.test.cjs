const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

const source = fs.readFileSync(
  path.join(__dirname, "../src/hooks/Lightbox/LightboxImage.tsx"),
  "utf8"
);
const ast = ts.createSourceFile(
  "LightboxImage.tsx",
  source,
  ts.ScriptTarget.Latest
);
const handlers = new Set([
  "getScrollMode",
  "onContainerScroll",
  "onLeftScroll",
  "onRightScroll",
  "onImageScrollPanY",
  "onImageScroll",
]);
const snippets = [];
function visit(node) {
  if (ts.isFunctionDeclaration(node) && handlers.has(node.name?.text)) {
    snippets.push(node.getText(ast));
  }
  if (ts.isVariableStatement(node)) {
    const name = node.declarationList.declarations[0].name.getText(ast);
    if (name.startsWith("SCROLL_") || name.startsWith("ZOOM_")) {
      snippets.push(node.getText(ast));
    }
  }
  ts.forEachChild(node, visit);
}
visit(ast);
// Execute the real wheel handlers with their closure state supplied by the
// fixture. No browser is needed, and production code stays in place.
const compiled = ts.transpileModule(snippets.join("\n"), {}).outputText;

function setup(overrides = {}) {
  const calls = { left: 0, right: 0, zoom: [], stopped: 0 };
  const state = {
    current: true,
    zoom: 1,
    defaultZoom: 1,
    positionY: 0,
    minMaxY: () => [-300, 0],
    scrollMode: "PAN_Y",
    scrollToChangeImage: true,
    scrollAttemptsBeforeChange: 2,
    scrollAttempts: { current: 0 },
    firstScroll: { current: null },
    inScrollGroup: { current: false },
    GQL: { ImageLightboxScrollMode: { PanY: "PAN_Y", Zoom: "ZOOM" } },
    onLeft: () => calls.left++,
    onRight: () => calls.right++,
    setZoom: (v) => calls.zoom.push(v),
    debouncedScrollReset() {},
    ...overrides,
  };
  state.setPositionY = (v) => {
    state.positionY = v;
  };
  vm.createContext(state);
  vm.runInContext(compiled, state);
  return {
    state,
    calls,
    wheel(deltaY, extra = {}, container = false) {
      const event = {
        deltaY,
        shiftKey: false,
        stopPropagation: () => calls.stopped++,
        ...extra,
      };
      if (container) state.onContainerScroll(event);
      else state.onImageScroll(event);
    },
  };
}

for (const delta of [8, 120]) {
  for (const [edge, positionY, direction] of [
    ["top", 0, -1],
    ["bottom", -300, 1],
  ]) {
    test("disabled navigation stays at " + edge + ", delta " + delta, () => {
      const { state, calls, wheel } = setup({
        scrollToChangeImage: false,
        positionY,
      });
      for (let i = 0; i < 6; i++) wheel(direction * delta);
      assert.equal(calls.left, 0);
      assert.equal(calls.right, 0);
      assert.equal(state.positionY, positionY);
      assert.equal(state.scrollAttempts.current, 0);
      assert.equal(calls.stopped, 6);
    });

    test("enabled navigation preserves " + edge + ", delta " + delta, () => {
      const { calls, wheel } = setup({ positionY });
      for (let i = 0; i < 3; i++) wheel(direction * delta);
      assert.equal(direction < 0 ? calls.left : calls.right, 1);
      assert.equal(direction < 0 ? calls.right : calls.left, 0);
    });
  }

  for (const direction of [-1, 1]) {
    test(
      "disabled navigation keeps a fitting image, delta " + direction * delta,
      () => {
        const { state, calls, wheel } = setup({
          scrollToChangeImage: false,
          minMaxY: () => [0, 0],
        });
        for (let i = 0; i < 6; i++) wheel(direction * delta);
        assert.equal(calls.left, 0);
        assert.equal(calls.right, 0);
        assert.equal(state.positionY, 0);
      }
    );

    test(
      "enabled navigation changes a fitting image, delta " + direction * delta,
      () => {
        const { calls, wheel } = setup({ minMaxY: () => [0, 0] });
        wheel(direction * delta);
        assert.equal(direction < 0 ? calls.left : calls.right, 1);
      }
    );
  }

  test("disabled navigation still pans and clamps, delta " + delta, () => {
    const { state, calls, wheel } = setup({
      scrollToChangeImage: false,
      positionY: -150,
    });
    wheel(delta);
    assert.equal(state.positionY, delta === 8 ? -154 : -225);
    for (let i = 0; i < 100; i++) wheel(delta);
    assert.equal(state.positionY, -300);
    for (let i = 0; i < 100; i++) wheel(-delta);
    assert.equal(state.positionY, 0);
    assert.equal(calls.left + calls.right, 0);
  });
}

test("disabled navigation also applies over the container", () => {
  const { state, calls, wheel } = setup({ scrollToChangeImage: false });
  for (let i = 0; i < 6; i++) wheel(-120, {}, true);
  assert.equal(calls.left + calls.right, 0);
  assert.equal(state.positionY, 0);
});

test("shift-to-Pan Y respects the option", () => {
  const { calls, wheel } = setup({
    scrollMode: "ZOOM",
    scrollToChangeImage: false,
  });
  for (let i = 0; i < 6; i++) wheel(-120, { shiftKey: true });
  assert.equal(calls.left + calls.right, 0);
  assert.deepEqual(calls.zoom, []);
});

test("Zoom and shift-to-Zoom are unchanged", () => {
  for (const scrollToChangeImage of [false, true]) {
    for (const delta of [8, 120]) {
      for (const scrollMode of ["ZOOM", "PAN_Y"]) {
        const { calls, wheel } = setup({ scrollMode, scrollToChangeImage });
        wheel(delta, { shiftKey: scrollMode === "PAN_Y" });
        assert.equal(calls.left + calls.right, 0);
        assert.equal(calls.zoom[0], delta === 8 ? 1 - delta / 700 : 1 / 1.1);
      }
    }
  }
});

test("infinite scrolling still waits for a new scroll group", () => {
  const { calls, wheel } = setup({ inScrollGroup: { current: true } });
  wheel(-8);
  assert.equal(calls.left, 0);
  wheel(-3);
  wheel(-8);
  assert.equal(calls.left, 1);
});
