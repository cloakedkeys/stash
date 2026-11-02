const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const ts = require("typescript");
const {
  ApolloClient,
  ApolloLink,
  InMemoryCache,
  Observable,
  gql,
} = require("@apollo/client");

const scenesQuery = gql`
  query FindScenes($scene_ids: [Int!]) {
    findScenes(scene_ids: $scene_ids) {
      count
      scenes {
        id
        title
        resume_time
        play_duration
      }
    }
  }
`;
const statsQuery = gql`
  query Stats {
    stats {
      total_play_duration
    }
  }
`;
const saveMutation = gql`
  mutation SceneSaveActivity(
    $id: ID!
    $resume_time: Float
    $playDuration: Float
  ) {
    sceneSaveActivity(
      id: $id
      resume_time: $resume_time
      playDuration: $playDuration
    )
  }
`;

// Exercise the service's real update callback with Apollo, without starting
// the browser client or rendering a React hook.
function saveActivityOptions() {
  const source = fs.readFileSync(
    path.join(__dirname, "../src/core/StashService.ts"),
    "utf8"
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
  });
  const module = { exports: {} };
  vm.runInNewContext(compiled.outputText, {
    module,
    exports: module.exports,
    require(name) {
      if (name.startsWith("@apollo/client")) return require(name);
      if (name === "./createClient") return { createClient: () => ({}) };
      if (name === "./generated-graphql") {
        return {
          FindScenesDocument: scenesQuery,
          useSceneSaveActivityMutation: (options) => options,
        };
      }
      return {};
    },
  });
  const createSaveActivity = module.exports.useSceneSaveActivity;
  return createSaveActivity();
}

for (const playDuration of [0, 10]) {
  test(`activity saves (${playDuration}s) keep watched scene lists cached`, async () => {
    const cache = new InMemoryCache();
    const scene = {
      __typename: "Scene",
      id: "1",
      title: "Playing scene",
      resume_time: 5,
      play_duration: 20,
    };
    const data = { findScenes: { count: 1, scenes: [scene] } };
    const lists = [{}, { scene_ids: [1] }];
    for (const variables of lists) {
      cache.writeQuery({ query: scenesQuery, variables, data });
    }
    cache.writeQuery({
      query: statsQuery,
      data: { stats: { total_play_duration: 100 } },
    });

    const requests = [];
    const client = new ApolloClient({
      cache,
      link: new ApolloLink((operation) => {
        requests.push(operation.operationName);
        return Observable.of({
          data:
            operation.operationName === "SceneSaveActivity"
              ? { sceneSaveActivity: true }
              : data,
        });
      }),
    });
    const subscriptions = lists.map((variables) =>
      client.watchQuery({ query: scenesQuery, variables }).subscribe({})
    );
    try {
      const { update } = saveActivityOptions();
      for (const resume_time of [30, 40]) {
        await client.mutate({
          mutation: saveMutation,
          variables: { id: "1", resume_time, playDuration },
          update,
        });
      }
      // Let Apollo dispatch any refetch triggered by an evicted query.
      await new Promise((resolve) => setTimeout(resolve, 0));
      assert.deepEqual(requests, ["SceneSaveActivity", "SceneSaveActivity"]);
      for (const variables of lists) {
        assert.deepEqual(cache.readQuery({ query: scenesQuery, variables }), {
          findScenes: {
            count: 1,
            scenes: [
              {
                ...scene,
                resume_time: 40,
                play_duration: 20 + 2 * playDuration,
              },
            ],
          },
        });
      }
      assert.deepEqual(cache.readQuery({ query: statsQuery }), {
        stats: { total_play_duration: 100 + 2 * playDuration },
      });
    } finally {
      for (const subscription of subscriptions) subscription.unsubscribe();
      client.stop();
    }
  });
}
