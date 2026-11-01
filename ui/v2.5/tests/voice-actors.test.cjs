const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { StaticRouter } = require("react-router-dom");
const { IntlProvider } = require("react-intl");

const actor = {
  id: "42",
  name: "Voice Artist",
  aliases: ["Voice Alias"],
  urls: ["https://example.com/voice"],
  details: "Animation and ADR credits",
  favorite: true,
  scene_count: 1,
  scenes: [{ id: "7", title: "Animation", paths: {}, files: [] }],
};

function render(location) {
  const hooks = {
    useFindVoiceActorsQuery: () => ({
      data: { findVoiceActors: { count: 1, voice_actors: [actor] } },
    }),
    useFindVoiceActorQuery: () => ({ data: { findVoiceActor: actor } }),
    useVoiceActorUserDataUpdateMutation: () => [() => {}, {}],
    useVoiceActorCreateMutation: () => [() => {}, {}],
    useVoiceActorUpdateMutation: () => [() => {}, {}],
    useVoiceActorDestroyMutation: () => [() => {}, {}],
  };
  const module = { exports: {} };
  const source = fs.readFileSync(
    path.join(__dirname, "../src/components/VoiceActors/VoiceActors.tsx"),
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
    require: (name) => {
      if (name === "src/core/generated-graphql") return hooks;
      if (name === "../Shared/LoadingIndicator")
        return { LoadingIndicator: () => null };
      if (name === "../Shared/FavoriteIcon")
        return { FavoriteIcon: () => null };
      if (name === "../Shared/ModifyRoute")
        return { ModifyRoute: require("react-router-dom").Route };
      if (name === "src/hooks/CurrentUser")
        return { useCanModify: () => true };
      return require(name);
    },
  });
  return renderToStaticMarkup(
    React.createElement(
      StaticRouter,
      { location },
      React.createElement(
        IntlProvider,
        {
          locale: "en",
          messages: { name: "Name", scenes: "Scenes", details: "Details" },
          onError() {},
        },
        React.createElement(module.exports.default)
      )
    )
  );
}

test("voice actor list links to separate details and creation", () => {
  const html = render("/voice-actors");
  assert.match(html, /href="\/voice-actors\/42"/);
  assert.match(html, /href="\/voice-actors\/new"/);
  assert.match(html, /Voice Alias/);
  assert.match(html, /1 voice actors/);
  assert.doesNotMatch(html, /href="\/performers/);
});

test("voice actor detail displays metadata and credited scenes", () => {
  const html = render("/voice-actors/42");
  assert.match(html, /Animation and ADR credits/);
  assert.match(html, /href="\/scenes\/7"/);
  assert.match(html, /href="\/voice-actors\/42\/edit"/);
  assert.match(html, /https:\/\/example.com\/voice/);
});

test("create and edit routes expose voice actor fields", () => {
  for (const location of ["/voice-actors/new", "/voice-actors/42/edit"]) {
    const html = render(location);
    for (const field of ["name", "details", "aliases", "urls", "favorite"]) {
      assert.match(html, new RegExp(`id="voice-actor-${field}"`));
    }
    assert.match(html, /type="submit"/);
  }
  assert.match(render("/voice-actors/42/edit"), /value="Voice Artist"/);
  assert.doesNotMatch(render("/voice-actors/new"), /value="Voice Artist"/);
});
