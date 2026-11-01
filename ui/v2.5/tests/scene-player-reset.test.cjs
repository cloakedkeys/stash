const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

function read(relative) {
  return fs.readFileSync(
    path.join(__dirname, "../src/components", relative),
    "utf8"
  );
}

function compile(source) {
  return ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      jsx: ts.JsxEmit.React,
      esModuleInterop: true,
    },
  }).outputText;
}

// Run the real reset effects and dependency lists without initializing media,
// casting, or the other browser-only plugins in ScenePlayer.
function playback() {
  const source = read("ScenePlayer/ScenePlayer.tsx");
  const ast = ts.createSourceFile(
    "ScenePlayer.tsx",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX
  );
  const effects = [];
  function visit(node) {
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(ast) === "useEffect"
    ) {
      const body = node.arguments[0].getText(ast);
      if (
        body.includes("player.playbackRate(") ||
        body.includes("player.loop(looping)")
      ) {
        effects.push(node.getText(ast));
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(ast);

  const state = {
    speed: 2,
    defaultSpeed: 2,
    loop: true,
    ab: { start: 12, end: 20, enabled: true, loopIfAfterEnd: true },
  };
  const player = {
    playbackRate: (value) => {
      state.speed = value;
    },
    defaultPlaybackRate: (value) => {
      state.defaultSpeed = value;
    },
    loop: (value) => {
      state.loop = value;
    },
    abLoopPlugin: {
      getOptions: () => state.ab,
      setOptions: (options) => {
        state.ab = options;
      },
    },
  };
  const getPlayer = () => player;
  const interactiveClient = {
    setLooping: (value) => {
      state.interactiveLoop = value;
    },
  };
  const previous = [];
  function render(id, looping = false) {
    let index = 0;
    vm.runInNewContext(compile(effects.join(";\n")), {
      getPlayer,
      interactiveClient,
      scene: { id },
      looping,
      useEffect(callback, dependencies) {
        const old = previous[index];
        previous[index++] = dependencies;
        if (!old || dependencies.some((value, i) => value !== old[i])) {
          callback();
        }
      },
    });
  }
  return { state, render };
}

test("scene changes reset speed and both enabled and staged A-B loops", () => {
  const { state, render } = playback();
  render("a");
  assert.equal(state.ab.enabled, false);
  assert.equal(state.ab.start, 0);
  assert.equal(state.ab.end, false);
  for (const id of ["b", "a"]) {
    state.speed = 1.5;
    state.defaultSpeed = 1.5;
    state.ab = { start: 7, end: false, enabled: false, loopIfAfterEnd: true };
    render(id);
    assert.equal(state.speed, 1);
    assert.equal(state.defaultSpeed, 1);
    assert.equal(state.ab.start, 0);
    assert.equal(state.ab.end, false);
    assert.equal(state.ab.enabled, false);
    assert.equal(state.ab.loopIfAfterEnd, true);
  }
});

test("scene changes restore an unchanged loop default", () => {
  for (const looping of [false, true]) {
    const { state, render } = playback();
    render("a", looping);
    state.loop = !looping;
    render("b", looping);
    assert.equal(state.loop, looping);
    assert.equal(state.interactiveLoop, looping);
  }
});

test("same-scene renders preserve playback choices", () => {
  const { state, render } = playback();
  render("a");
  state.speed = 1.5;
  state.loop = true;
  state.ab = { start: 7, end: 15, enabled: true };
  render("a");
  assert.equal(state.speed, 1.5);
  assert.equal(state.loop, true);
  assert.equal(state.ab.start, 7);
  assert.equal(state.ab.enabled, true);
});

// A small hook runner exercises the real panel's state and generated styles.
// DOM lookups are replaced with a video style attribute; no browser is needed.
function filters() {
  const values = [];
  let index = 0;
  let dirty = false;
  const style = { value: "" };
  const video = { tagName: "VIDEO", attributes: { getNamedItem: () => style } };
  const React = {
    createElement: (type, props, ...children) => ({
      type,
      props: { ...props, children },
    }),
    useState(initial) {
      const slot = index++;
      if (!(slot in values)) values[slot] = initial;
      return [
        values[slot],
        (value) => {
          if (values[slot] !== value) {
            values[slot] = value;
            dirty = true;
          }
        },
      ];
    },
  };
  const module = { exports: {} };
  const source = read("Scenes/SceneDetails/SceneVideoFilterPanel.tsx");
  vm.runInNewContext(compile(source), {
    module,
    exports: module.exports,
    require(name) {
      if (name === "react") return React;
      if (name === "react-intl") {
        return { useIntl: () => ({ formatMessage: ({ id }) => id }) };
      }
      if (name.endsWith("/util")) return { VIDEO_PLAYER_ID: "video" };
      return {};
    },
    document: {
      getElementById: (id) =>
        id === "video"
          ? {
              getElementsByTagName: (tag) => (tag === "video" ? [video] : []),
            }
          : null,
    },
  });
  function render(id) {
    let tree;
    do {
      dirty = false;
      index = 0;
      tree = module.exports.SceneVideoFilterPanel({ scene: { id } });
    } while (dirty);
    return tree.props.children.filter((child) => child?.props?.range);
  }
  return { render, style };
}

test("all filters, transforms and applied styles reset on A → B → A", () => {
  const { render, style } = filters();
  for (const [from, to] of [
    ["a", "b"],
    ["b", "a"],
  ]) {
    const sliders = render(from);
    assert.equal(sliders.length, 13);
    for (const slider of sliders) {
      slider.props.setValue(slider.props.range.default + 1);
    }
    const changed = render(from);
    assert.ok(
      changed.every(({ props }) => props.value !== props.range.default)
    );
    assert.match(style.value, /brightness\(/);
    assert.match(style.value, /rotate\(/);
    assert.match(style.value, /scale\(/);
    assert.match(style.value, /url\(#videoFilter\)/);
    const reset = render(to);
    assert.ok(reset.every(({ props }) => props.value === props.range.default));
    assert.equal(style.value, "filter:; transform:;");
  }
});
