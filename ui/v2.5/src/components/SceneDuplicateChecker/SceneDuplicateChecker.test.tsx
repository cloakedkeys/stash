import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { CriterionModifier } from "src/core/generated-graphql";
import {
  CriterionValue,
  ModifierCriterion,
} from "src/models/list-filter/criteria/criterion";
import { ListFilterModel } from "src/models/list-filter/filter";
import { CriterionType } from "src/models/list-filter/types";
import { SceneDuplicateChecker } from "./SceneDuplicateChecker";

const page = vi.hoisted(() => ({
  search: "",
  push: vi.fn(),
  duplicates: vi.fn(),
}));

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({
    location: { search: page.search },
    push: page.push,
  }),
  Link: () => null,
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: {} }),
}));
vi.mock("src/core/generated-graphql", async (importOriginal) => ({
  ...(await importOriginal<typeof import("src/core/generated-graphql")>()),
  useFindDuplicateScenesQuery: (options: unknown) => {
    page.duplicates(options);
    // still loading: the page stops at the spinner, after the query is made
    return { data: undefined, loading: true, refetch: vi.fn() };
  },
  useFindScenesQuery: () => ({ data: undefined }),
}));

function render() {
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={{}} onError={() => {}}>
      <SceneDuplicateChecker />
    </IntlProvider>
  );
}

function sentVariables(search: string) {
  page.search = search;
  render();
  expect(page.duplicates).toHaveBeenCalled();
  const options = page.duplicates.mock.calls.at(-1)![0] as {
    variables: GQL.FindDuplicateScenesQueryVariables;
  };
  return options.variables;
}

// Builds a link the way the scenes list does, so a filter copied from there
// works here too.
function sceneListQuery(
  ...criteria: [CriterionType, CriterionModifier, CriterionValue][]
) {
  const filter = new ListFilterModel(GQL.FilterMode.Scenes);
  for (const [type, modifier, value] of criteria) {
    const c = filter.makeCriterion(type) as ModifierCriterion<CriterionValue>;
    c.modifier = modifier;
    c.value = value;
    filter.criteria.push(c);
  }
  return "?" + filter.makeQueryParameters();
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("duplicate checker scene filter", () => {
  it("sends no scene filter when the address has none", () => {
    const variables = sentVariables("?distance=4&durationDiff=5");
    expect(variables).toEqual({
      distance: 4,
      duration_diff: 5,
      scene_filter: undefined,
    });
  });

  it("sends the organized criterion from the address", () => {
    const search = sceneListQuery([
      "organized",
      CriterionModifier.Equals,
      "false",
    ]);
    expect(sentVariables(search).scene_filter).toEqual({ organized: false });
  });

  it("sends a path that excludes a folder, with characters that need escaping", () => {
    const search = sceneListQuery([
      "path",
      CriterionModifier.Excludes,
      "/media/deep fakes & {edits}=+#;?",
    ]);
    expect(sentVariables(search).scene_filter).toEqual({
      path: {
        modifier: CriterionModifier.Excludes,
        value: "/media/deep fakes & {edits}=+#;?",
      },
    });
  });

  it("sends an excluded tag together with a minimum duration", () => {
    const search = sceneListQuery(
      [
        "tags",
        CriterionModifier.Excludes,
        {
          items: [{ id: "12", label: "Not a duplicate" }],
          excluded: [],
          depth: 0,
        },
      ],
      [
        "duration",
        CriterionModifier.GreaterThan,
        { value: 10, value2: undefined },
      ]
    );
    expect(sentVariables(search).scene_filter).toEqual({
      tags: {
        modifier: CriterionModifier.Excludes,
        value: ["12"],
        excludes: [],
        depth: 0,
      },
      duration: {
        modifier: CriterionModifier.GreaterThan,
        value: 10,
        value2: undefined,
      },
    });
  });
});
