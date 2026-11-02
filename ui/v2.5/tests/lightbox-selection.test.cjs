const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

// Run the component's state, effects and handlers without a DOM. Child
// components remain element descriptions; media and layout are not exercised.
function setup(overrides = {}) {
  const slots = [];
  const listeners = new Map();
  let cursor = 0;
  let dirty = false;
  let effects = [];
  let tree;
  let props = {
    images: images("a", "b", "c"),
    initialIndex: 1,
    isVisible: true,
    isLoading: false,
    showNavigation: true,
    hide: () => assert.fail("unexpected close"),
    ...overrides,
  };
  const react = {
    createElement: (type, props, ...children) => ({
      type,
      props: { ...props, children },
    }),
    useState(initial) {
      const slot = cursor++;
      if (!(slot in slots)) slots[slot] = initial;
      return [
        slots[slot],
        (value) => {
          const next = typeof value === "function" ? value(slots[slot]) : value;
          if (!Object.is(next, slots[slot])) {
            slots[slot] = next;
            dirty = true;
          }
        },
      ];
    },
    useRef(initial) {
      const slot = cursor++;
      slots[slot] ??= { current: initial };
      return slots[slot];
    },
    useMemo(fn, deps) {
      const slot = cursor++;
      if (!slots[slot] || deps.some((v, i) => v !== slots[slot].deps[i])) {
        slots[slot] = { value: fn(), deps };
      }
      return slots[slot].value;
    },
    useCallback(fn, deps) {
      return react.useMemo(() => fn, deps);
    },
    useEffect(fn, deps) {
      const slot = cursor++;
      if (!slots[slot] || deps.some((v, i) => v !== slots[slot].deps[i])) {
        effects.push(() => {
          slots[slot]?.cleanup?.();
          slots[slot] = { deps, cleanup: fn() };
        });
      }
    },
  };
  class Mousetrap {
    static pause() {}
    static unpause() {}
    reset() {}
    bind() {}
    unbind() {}
  }
  const noop = () => {};
  const modules = {
    react: { ...react, default: react, __esModule: true },
    mousetrap: Mousetrap,
    classnames: (name) => name,
    "../Interval": () => [noop, noop],
    "../PageVisibility": noop,
    "../keybinds": { useRatingKeybinds: noop },
    "../debounce": { useDebounce: () => Object.assign(noop, { cancel: noop }) },
    "../Toast": { useToast: () => ({}) },
    "../Config": { useConfigurationContext: () => ({}) },
    "../LocalForage": { useInterfaceLocalForage: () => [{}, noop] },
    "react-intl": { useIntl: () => ({ formatMessage: () => "" }) },
    "src/core/StashService": { useImageUserDataUpdate: () => [noop] },
    "src/core/enums": { imageLightboxDisplayModeIntlMap: new Map() },
    "src/core/files": { imageTitle: (image) => image.id },
    "src/utils/visualFile": { isVideo: () => false },
    "src/core/generated-graphql": {
      ImageLightboxScrollMode: {},
      ImageLightboxDisplayMode: {},
    },
  };
  const module = { exports: {} };
  const source = fs.readFileSync(
    path.join(__dirname, "../src/hooks/Lightbox/Lightbox.tsx"),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      jsx: ts.JsxEmit.React,
      esModuleInterop: true,
    },
  });
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require: (name) => modules[name] ?? new Proxy({}, { get: () => noop }),
    document: {
      body: { style: {} },
      addEventListener: (name, fn) => listeners.set(name, fn),
      removeEventListener: (name) => listeners.delete(name),
    },
  });
  function render(update = {}) {
    props = { ...props, ...update };
    for (let pass = 0; pass < 20; pass++) {
      dirty = false;
      cursor = 0;
      effects = [];
      tree = module.exports.default(props);
      if (dirty) continue;
      for (const effect of effects) effect();
      if (!dirty) return;
    }
    assert.fail("render did not settle");
  }
  function nodes(node) {
    if (!node || typeof node !== "object") return [];
    if (Array.isArray(node)) return node.flatMap(nodes);
    return [node, ...nodes(node.props?.children)];
  }
  render();
  return {
    render,
    id: () =>
      nodes(tree).find((n) => n.props?.className === "image-link")?.props.to,
    key(key) {
      listeners.get("keydown")({ key });
      render();
    },
    jump(last) {
      const nav = nodes(tree).find(
        (n) => n.props?.className === "Lightbox-nav"
      );
      const buttons = nav.props.children;
      buttons[last ? 0 : buttons.length - 1].props.onClick();
      render();
    },
  };
}

function images(...ids) {
  return ids.map((id) => ({ id, paths: { image: id, preview: "" } }));
}

test("follows the viewed id through repeated reorders", () => {
  const viewer = setup();
  viewer.render({ images: images("b", "a", "c") });
  assert.equal(viewer.id(), "/images/b");
  viewer.render({ images: images("a", "c", "b") });
  assert.equal(viewer.id(), "/images/b");
  viewer.key("ArrowLeft");
  assert.equal(viewer.id(), "/images/c");
  viewer.render({ images: images("c", "b", "a") });
  assert.equal(viewer.id(), "/images/c");
});

test("keeps position if the image leaves and follows its replacement", () => {
  const viewer = setup();
  viewer.render({ images: images("a", "c", "d") });
  assert.equal(viewer.id(), "/images/c");
  viewer.render({ images: images("c", "a", "d") });
  assert.equal(viewer.id(), "/images/c");
});

test("clamps a missing image's position when the page shrinks", () => {
  const viewer = setup({ initialIndex: 2 });
  viewer.render({ images: images("a", "b") });
  assert.equal(viewer.id(), "/images/b");
});

test("images without ids keep their position", () => {
  const viewer = setup({ images: images(undefined, undefined, undefined) });
  viewer.render({ images: images("a", "b", "c") });
  assert.equal(viewer.id(), "/images/b");
});

for (const direction of ["ArrowRight", "ArrowLeft"]) {
  test(`page crossing and first/last jumps after ${direction}`, () => {
    const calls = [];
    const viewer = setup({
      initialIndex: direction === "ArrowRight" ? 2 : 0,
      page: direction === "ArrowRight" ? 1 : 2,
      pageCallback: (arg) => calls.push(arg),
    });
    viewer.key(direction);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].direction, direction === "ArrowRight" ? 1 : -1);
    // An old id is still present: explicit page intent must win.
    viewer.render({
      images: images("x", "a", "c", "y"),
      page: direction === "ArrowRight" ? 2 : 1,
    });
    assert.equal(
      viewer.id(),
      direction === "ArrowRight" ? "/images/x" : "/images/y"
    );
    viewer.jump(false);
    assert.equal(viewer.id(), "/images/x");
    viewer.jump(true);
    assert.equal(viewer.id(), "/images/y");
    viewer.render({ images: images("y", "x", "a", "c") });
    assert.equal(viewer.id(), "/images/y");
  });
}
