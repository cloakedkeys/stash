const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");

function panel(showChildStudioContent) {
  const source = fs.readFileSync(
    path.join(
      __dirname,
      "../src/components/Studios/StudioDetails/StudioPerformersPanel.tsx"
    ),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
      esModuleInterop: true,
    },
  });
  const module = { exports: {} };
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require: (name) => {
      if (name === "src/core/studios")
        return { useStudioFilterHook: () => () => {} };
      if (name === "src/components/Performers/PerformerList")
        return { FilteredPerformerList: () => null };
      if (name === "src/models/list-filter/criteria/studios")
        return { StudiosCriterion: class {} };
      if (name === "src/components/List/views")
        return { View: { StudioPerformers: "studio_performers" } };
      return require(name);
    },
  });
  return module.exports.StudioPerformersPanel({
    active: true,
    studio: { id: "7", name: "Current studio" },
    showChildStudioContent,
  }).props;
}

for (const children of [false, true]) {
  test(
    children
      ? "studio performer counts include descendants when requested"
      : "studio performer counts stay within the current studio",
    () => {
      const props = panel(children);
      assert.equal(props.studioScope.id, "7");
      assert.equal(props.studioScope.depth, children ? -1 : 0);
      for (const media of ["scenes", "images", "galleries", "groups"]) {
        const criterion = props.extraCriteria[media][0];
        assert.equal(criterion.value.items[0].id, "7");
        assert.equal(criterion.value.depth, props.studioScope.depth);
      }
    }
  );
}

test("performer queries pass the same studio scope to filtering and card counts", () => {
  const source = fs.readFileSync(
    path.join(__dirname, "../src/core/StashService.ts"),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, esModuleInterop: true },
  });
  const module = { exports: {} };
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require: (name) => {
      if (name === "./generated-graphql")
        return { useFindPerformersQuery: (options) => options };
      if (name === "./createClient")
        return {
          createClient: () => ({ client: { query: (options) => options } }),
        };
      return {};
    },
  });
  const scope = { id: "7", depth: -1 };
  const filter = {
    makeFindFilter: () => ({ sort: "scenes_count" }),
    makeFilter: () => ({ scene_count: { value: 1, modifier: "EQUALS" } }),
  };
  for (const query of [
    module.exports.useFindPerformers,
    module.exports.queryFindPerformers,
  ]) {
    const { variables } = query(filter, scope);
    assert.equal(variables.studio_scope, scope);
    assert.equal(variables.performer_filter.studio_scope, scope);
    assert.equal(variables.performer_filter.scene_count.value, 1);
  }
});

test("cached studio counts stay separate from other studios and global counts", () => {
  const { parse, print } = require("graphql");
  const { InMemoryCache, gql } = require("@apollo/client/core");
  const fragments = parse(
    fs.readFileSync(
      path.join(__dirname, "../graphql/data/performer.graphql"),
      "utf8"
    )
  );
  const fragment = fragments.definitions.find(
    (definition) => definition.name?.value === "PerformerListData"
  );
  assert.ok(fragment);
  const counts = fragment.selectionSet.selections.filter(
    (field) => field.kind === "Field" && field.name.value.endsWith("_count")
  );
  const fields = counts.map(print).join(" ");
  const scopedQuery = gql(
    `query Counts($studio_scope: StudioScopeInput) { findPerformer(id: "1") { id ${fields} } }`
  );
  const globalQuery = gql(
    `query { findPerformer(id: "1") { id scene_count audio_count image_count gallery_count group_count } }`
  );
  const cache = new InMemoryCache();
  function write(query, variables, value) {
    cache.writeQuery({
      query,
      variables,
      data: {
        findPerformer: {
          __typename: "Performer",
          id: "1",
          scene_count: value,
          audio_count: value,
          image_count: value,
          gallery_count: value,
          group_count: value,
        },
      },
    });
  }
  const first = { studio_scope: { id: "7", depth: 0 } };
  const second = { studio_scope: { id: "8", depth: 0 } };
  const children = { studio_scope: { id: "7", depth: -1 } };
  write(scopedQuery, first, 1);
  write(scopedQuery, second, 2);
  write(scopedQuery, children, 3);
  write(globalQuery, {}, 10);
  for (const [variables, expected] of [
    [first, 1],
    [second, 2],
    [children, 3],
  ]) {
    assert.equal(
      cache.readQuery({ query: scopedQuery, variables }).findPerformer
        .scene_count,
      expected
    );
  }
  assert.equal(
    cache.readQuery({ query: globalQuery }).findPerformer.scene_count,
    10
  );
});
