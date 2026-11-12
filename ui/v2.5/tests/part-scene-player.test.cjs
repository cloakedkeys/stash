const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");

const read = (file) =>
  fs.readFileSync(path.join(__dirname, "../src", file), "utf8");
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
function load(file, requireMock) {
  const module = { exports: {} };
  vm.runInNewContext(compile(read(file)), {
    module,
    exports: module.exports,
    require: requireMock,
  });
  return module.exports;
}
const utils = load("utils/scenePlayback.ts", () => ({}));
const playerSource = read("components/ScenePlayer/ScenePlayer.tsx");
function find(source, predicate) {
  const ast = ts.createSourceFile(
    "component.tsx",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX
  );
  const found = [];
  function visit(node) {
    if (predicate(node, ast)) found.push(node.getText(ast));
    ts.forEachChild(node, visit);
  }
  visit(ast);
  assert.equal(found.length, 1);
  return found[0];
}
const variable = (source, name) =>
  "const " +
  find(
    source,
    (n) => ts.isVariableDeclaration(n) && n.name.getText() === name
  ) +
  ";";
const effect = (source, text) =>
  find(
    source,
    (n, ast) =>
      ts.isCallExpression(n) &&
      n.expression.getText(ast) === "useEffect" &&
      n.arguments[0].getText(ast).includes(text)
  );
function playback(scene, initial = 0, loaded) {
  const state = { time: 0, sources: [], loop: false, thumbnail: "unset" };
  const player = {
    trackActivity: () => ({ reset() {} }),
    mobileUi() {},
    sourceSelector: () => ({
      setSources(sources) {
        state.sources = sources;
      },
      setShouldAutoplay() {},
    }),
    load() {},
    focus() {},
    autostartButton: () => ({ getEnabled: () => false }),
    ready: (fn) => fn(),
    vttThumbnails: () => ({
      src: (value) => {
        state.thumbnail = value;
      },
    }),
    currentTime(value) {
      if (value !== undefined) state.time = value;
      return state.time;
    },
    loop(value) {
      state.loop = value;
    },
  };
  const context = {
    ...utils,
    scene,
    file: scene.files[0],
    forceLoop: false,
    permitLoop: true,
    maxLoopDuration: 100,
    useMemo: (fn) => fn(),
    useEffect: (fn) => fn(),
    getPlayer: () => player,
    loadedSource: { current: loaded },
    fingerprintsMatch: () => true,
    setReady() {},
    interactiveClient: { pause() {}, setLooping() {} },
    UAParser: () => ({ browser: {} }),
    uiConfig: {},
    interfaceConfig: {},
    _initialTimestamp: initial,
    setTime: (time) => {
      state.time = time;
    },
    auto: {},
    autoplay: false,
    autostartIntent: {},
    started: {},
    URL,
    state,
  };
  const code = [
    "const isPart = isPartScene(scene);",
    playerSource.includes("const duration = scenePlaybackDuration")
      ? "const " +
        find(
          playerSource,
          (n) =>
            ts.isVariableDeclaration(n) &&
            n.name.getText() === "duration" &&
            n.initializer?.getText().startsWith("scenePlaybackDuration")
        ) +
        ";"
      : "const duration = file.duration;",
    variable(playerSource, "looping"),
    effect(playerSource, "sourceSelector.setSources"),
    effect(playerSource, "player.loop(looping)"),
  ].join("\n");
  vm.runInNewContext(compile(code), context);
  return state;
}
function scene(start_time, end_time, resume_time = 0) {
  return {
    id: "1",
    start_time,
    end_time,
    resume_time,
    interactive: true,
    paths: {
      vtt: "https://stash/sprite.vtt",
      caption: "https://stash/caption",
      funscript: "https://stash/script",
    },
    files: [{ duration: 600, fingerprints: [], width: 640, height: 480 }],
    sceneStreams: [
      { url: "https://stash/scene/1/stream.mp4" },
      { url: "https://stash/scene/1/stream.m3u8" },
    ],
  };
}
for (const [name, start, end, duration] of [
  ["bounded", 100, 140, 40],
  ["start only", 560, null, 40],
  ["end only", null, 40, 40],
  ["whole file", null, null, 600],
]) {
  test(`${name}: sources, resume, loop and activity use the playback length`, () => {
    const s = scene(start, end, 50);
    const state = playback(s);
    assert.deepEqual(
      Array.from(state.sources, (s) => s.duration),
      [duration, duration]
    );
    assert.equal(state.time, duration === 40 ? 0 : 50);
    assert.equal(state.loop, duration === 40);
    assert.equal(playback({ ...s, resume_time: 20 }).time, 20);
    // Run the real middleware, then use that duration in the real activity plugin.
    let middleware;
    load("components/ScenePlayer/live.ts", (name) =>
      name === "lodash-es"
        ? { debounce: (fn) => fn }
        : {
            use: (_, fn) => {
              middleware = fn;
            },
          }
    );
    const live = middleware({});
    live.setSource(state.sources[0], () => {});
    const playerDuration = live.duration(999);
    const Activity = load("components/ScenePlayer/track-activity.ts", () => ({
      getPlugin: () => class {},
      registerPlugin() {},
    })).default;
    const activity = Object.assign(Object.create(Activity.prototype), {
      enabled: true,
      totalPlayDuration: 20,
      currentPlayDuration: 20,
      minimumPlayPercent: 50,
      playCountIncremented: false,
      player: {
        duration: () => playerDuration,
        currentTime: () => duration * 0.99,
      },
      incrementPlayCount() {
        this.count = (this.count ?? 0) + 1;
      },
      saveActivity(resume, played) {
        this.saved = { resume, played };
      },
    });
    activity.sendActivity();
    assert.equal(activity.count ?? 0, duration === 40 ? 1 : 0);
    assert.deepEqual(activity.saved, { resume: 0, played: 20 });
    assert.equal(
      state.thumbnail,
      start != null || end != null ? null : s.paths.vtt
    );
  });
}
test("changing saved bounds reloads the same scene and rejects an old out-of-range timestamp", () => {
  const s = scene(100, 140);
  const loaded = {
    sceneId: s.id,
    fingerprints: [],
    startTime: 100,
    endTime: 180,
  };
  const state = playback(s, 50, loaded);
  assert.equal(state.sources.length, 2);
  assert.equal(state.time, 0);
});

test("part captions are not queried or installed and scripts are not uploaded; whole-file controls remain enabled", () => {
  for (const isPart of [true, false]) {
    const s = scene(isPart ? 100 : null, isPart ? 140 : null);
    let query;
    let uploads = 0;
    let tracks = 0;
    const selector = {
      textTracks: [],
      addTextTrack() {
        tracks++;
      },
      selectTextTrack() {},
    };
    const context = {
      ...utils,
      scene: s,
      isPart,
      interactive: !isPart && s.interactive,
      GQL: {
        useSceneEmbeddedCaptionsQuery(options) {
          query = options;
          return { data: { findScene: { id: s.id, embedded_captions: [] } } };
        },
      },
      useEffect: (fn) => fn(),
      getPlayer: () => ({ sourceSelector: () => selector }),
      loadedSource: { current: { sceneId: s.id } },
      interactiveInitialised: true,
      uploadScript: () => {
        uploads++;
        return Promise.resolve();
      },
      interactiveReady: {},
      window: {
        navigator: { language: "en" },
        location: { href: "https://stash/" },
      },
      URL,
      canonicalLanguageCode: (s) => s,
      languageName: (s) => s,
    };
    s.captions = [{ language_code: "en", caption_type: "srt" }];
    const queryDeclaration = find(
      playerSource,
      (n) =>
        ts.isVariableDeclaration(n) &&
        n.name.getText().includes("data: embeddedCaptions")
    );
    vm.runInNewContext(
      compile(
        "const " +
          queryDeclaration +
          ";\n" +
          effect(playerSource, "uploadScript(") +
          "\n" +
          effect(playerSource, "function getDefaultLanguageCode")
      ),
      context
    );
    assert.equal(!!query.skip, isPart);
    assert.equal(uploads, isPart ? 0 : 1);
    assert.equal(tracks, isPart ? 0 : 1);
  }
});

test("scrubber seeks on the scene timeline and hides part sprites and heatmap", () => {
  const source = read("components/ScenePlayer/ScenePlayerScrubber.tsx");
  for (const isPart of [true, false]) {
    const s = scene(isPart ? 100 : null, isPart ? 140 : null);
    s.interactive_speed = 60;
    s.paths.interactive_heatmap = "https://stash/heatmap.png";
    s.scene_markers = [
      { seconds: 20, title: "Middle", primary_tag: { name: "Tag" } },
    ];
    const duration = isPart ? 40 : 600;
    let seek;
    const context = {
      ...utils,
      scene: s,
      file: s.files[0],
      duration,
      scrubWidth: 100,
      useCallback: (fn) => fn,
      sliderEl: { current: { clientWidth: 100, style: {} } },
      indicatorEl: { current: { style: {} } },
      position: {},
      onSeek: (time) => {
        seek = time;
      },
    };
    vm.runInNewContext(
      compile(variable(source, "setPosition") + "\nsetPosition(0, true);"),
      context
    );
    assert.equal(seek, duration / 2);
    let vtt;
    const hooks = {
      ...React,
      useRef: () => ({ current: null }),
      useState: (initial) => [
        typeof initial === "number" ? 300 : initial,
        () => {},
      ],
      useCallback: (fn) => fn,
      useEffect() {},
    };
    const { ScenePlayerScrubber } = load(
      "components/ScenePlayer/ScenePlayerScrubber.tsx",
      (name) => {
        if (name === "react") return hooks;
        if (name === "src/utils/scenePlayback") return utils;
        if (name === "src/hooks/sprite")
          return {
            useSpriteInfo: (url) => {
              vtt = url;
            },
          };
        if (name === "react-bootstrap")
          return {
            Button: ({ children }) =>
              React.createElement("button", {}, children),
          };
        if (name.endsWith("Icon")) return { Icon: () => null };
        return {};
      }
    );
    const html = renderToStaticMarkup(
      React.createElement(ScenePlayerScrubber, {
        scene: s,
        file: s.files[0],
        time: 0,
        onSeek() {},
        onScroll() {},
      })
    );
    assert.equal(vtt, isPart ? undefined : s.paths.vtt);
    assert.equal(html.includes("heatmap.png"), !isPart);
    if (isPart) assert.ok(html.includes("left:150px"));
  }
});

test("cover at current time sends file time and keeps the default cover request unchanged", async () => {
  const source = read("components/Scenes/SceneDetails/Scene.tsx");
  const fn = find(
    source,
    (n) =>
      ts.isFunctionDeclaration(n) &&
      n.name?.getText() === "onGenerateScreenshot"
  );
  for (const start of [100, null]) {
    let request;
    const context = {
      ...utils,
      scene: scene(start, start == null ? null : 140),
      generateScreenshot: async (value) => {
        request = value;
        return { data: {} };
      },
      Toast: {
        success() {},
        error(e) {
          throw e;
        },
      },
      intl: { formatMessage() {} },
      setScreenshotJobID() {},
    };
    vm.runInNewContext(compile(fn), context);
    await context.onGenerateScreenshot(12.5);
    assert.equal(request.variables.at, (start ?? 0) + 12.5);
    await context.onGenerateScreenshot();
    assert.equal(request.variables.at, undefined);
  }
});

test("part player hides interactive status and never restarts a connected script", () => {
  const returned = find(
    playerSource,
    (n) =>
      ts.isReturnStatement(n) &&
      n.expression?.getText().includes('className={cx("VideoPlayer"')
  );
  for (const isPart of [true, false]) {
    let plays = 0;
    const s = scene(isPart ? 100 : null, isPart ? 140 : null);
    const context = {
      ...utils,
      React,
      scene: s,
      isPart,
      interactive: !isPart,
      interactiveState: "ready",
      ConnectionState: { Ready: "ready" },
      getPlayer: () => ({ paused: () => true, currentTime: () => 12 }),
      interactiveClient: {
        ensurePlaying() {
          plays++;
        },
      },
      useEffect: (fn) => fn(),
      isPortrait: false,
      file: s.files[0],
      cx: () => "VideoPlayer",
      onKeyDown() {},
      videoRef: {},
      showScrubber: false,
      SceneInteractiveStatus: () =>
        React.createElement("span", {}, "interactive status"),
    };
    const element = vm.runInNewContext(
      compile(`(() => { ${returned} })()`),
      context
    );
    assert.equal(
      renderToStaticMarkup(element).includes("interactive status"),
      !isPart
    );
    context.getPlayer = () => ({ paused: () => false, currentTime: () => 12 });
    vm.runInNewContext(
      compile(effect(playerSource, "interactiveClient.ensurePlaying")),
      context
    );
    assert.equal(plays, isPart ? 0 : 1);
  }
});

test("file info hides part funscript and speed controls while whole files keep them", () => {
  const source = read("components/Scenes/SceneDetails/SceneFileInfoPanel.tsx");
  const functions = ["renderFunscript", "renderInteractiveSpeed"]
    .map((name) =>
      find(
        source,
        (n) => ts.isFunctionDeclaration(n) && n.name?.getText() === name
      )
    )
    .join("\n");
  for (const isPart of [true, false]) {
    const s = scene(isPart ? 100 : null, isPart ? 140 : null);
    s.interactive_speed = 60;
    const context = {
      ...utils,
      React,
      props: { scene: s },
      URLField: () => null,
      TextField: () => null,
      FormattedNumber: () => null,
    };
    vm.runInNewContext(compile(functions), context);
    assert.equal(context.renderFunscript() == null, isPart);
    assert.equal(context.renderInteractiveSpeed() == null, isPart);
  }
});
