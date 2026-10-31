const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

// Run the real plugin with only video.js's DOM components replaced. This keeps
// the error/fallback behavior testable without loading media in a browser.
class Component {
  constructor(player) {
    this.owner = player;
  }
  player() {
    return this.owner;
  }
  on() {}
  addClass() {}
  selected() {}
}

function setup(fetch) {
  const videojs = {
    getComponent: () => Component,
    getPlugin: () =>
      class {
        constructor(player) {
          this.player = player;
        }
      },
    registerComponent() {},
    registerPlugin() {},
  };
  const module = { exports: {} };
  const source = fs.readFileSync(
    path.join(__dirname, "../src/components/ScenePlayer/source-selector.ts"),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
  });
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require: () => videojs,
    MediaError: { MEDIA_ERR_SRC_NOT_SUPPORTED: 4, MEDIA_ERR_DECODE: 3 },
    fetch,
    console: { log() {} },
  });

  const events = new Map();
  const player = {
    sources: [],
    disposed: false,
    on(name, fn) {
      events.set(name, fn);
    },
    one() {},
    src(source) {
      this.sources.push(source);
    },
    error: () => ({ code: 4 }),
    currentSource() {
      return this.sources.at(-1);
    },
    currentTime: () => 0,
    isDisposed() {
      return this.disposed;
    },
    load() {},
    fail: () => events.get("error")(),
  };
  const plugin = new module.exports.default(player);
  function setScene(id) {
    plugin.setSources(
      [{ src: `${id}/direct` }, { src: `${id}/mp4` }, { src: `${id}/webm` }],
      undefined,
      undefined,
      `${id}/stream`
    );
  }
  setScene("first");
  return { player, setScene };
}

test("missing file stops codec fallback after one HEAD check", async () => {
  const calls = [];
  const { player } = setup(async (...args) => {
    calls.push(args);
    return { status: 404 };
  });
  await player.fail();
  await player.fail();
  assert.equal(player.sources.length, 1);
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "first/stream");
  assert.equal(calls[0][1].method, "HEAD");
  assert.equal(calls[0][1].cache, "no-store");
});

test("existing unsupported file advances once for repeated pending errors", async () => {
  let resolve;
  let checks = 0;
  const { player } = setup(() => {
    checks++;
    return new Promise((done) => {
      resolve = done;
    });
  });
  const first = player.fail();
  const repeated = player.fail();
  assert.equal(checks, 1);
  assert.equal(player.sources.length, 1);
  resolve({ status: 200 });
  await Promise.all([first, repeated]);
  assert.equal(player.sources.length, 2);
  assert.equal(player.currentSource().src, "first/mp4");
  await player.fail();
  assert.equal(player.currentSource().src, "first/webm");
  assert.equal(checks, 1);
});

test("a failed availability request preserves codec fallback", async () => {
  const { player } = setup(async () => {
    throw new Error("network unavailable");
  });
  await player.fail();
  assert.equal(player.currentSource().src, "first/mp4");
});

test("a stale HEAD response cannot change a new scene", async () => {
  let resolve;
  const { player, setScene } = setup(
    () =>
      new Promise((done) => {
        resolve = done;
      })
  );
  const pending = player.fail();
  setScene("second");
  resolve({ status: 200 });
  await pending;
  assert.equal(player.sources.length, 2);
  assert.equal(player.currentSource().src, "second/direct");
});

test("a disposed player cannot advance after its HEAD response", async () => {
  let resolve;
  const { player } = setup(
    () =>
      new Promise((done) => {
        resolve = done;
      })
  );
  const pending = player.fail();
  player.disposed = true;
  resolve({ status: 200 });
  await pending;
  assert.equal(player.sources.length, 1);
});
