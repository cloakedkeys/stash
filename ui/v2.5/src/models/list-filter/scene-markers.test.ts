import { describe, expect, it, vi } from "vitest";
import {
  CriterionModifier,
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { ListFilterModel } from "./filter";
import { ModifierCriterion, CriterionValue } from "./criteria/criterion";
import { CriterionType } from "./types";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

function markerCriterion(
  filter: ListFilterModel,
  type: CriterionType,
  modifier: CriterionModifier,
  value: CriterionValue
) {
  const c = filter.makeCriterion(type) as ModifierCriterion<CriterionValue>;
  c.modifier = modifier;
  c.value = value;
  return c;
}

function makeMarkerFilter() {
  const filter = new ListFilterModel(FilterMode.SceneMarkers);
  filter.criteria = [
    markerCriterion(filter, "tags", CriterionModifier.Includes, {
      items: [{ id: "1", label: "Tag" }],
      excluded: [],
      depth: 0,
    }),
    markerCriterion(filter, "scene_path", CriterionModifier.Includes, "/clips"),
    markerCriterion(
      filter,
      "scene_performer_favorite",
      CriterionModifier.Equals,
      "true"
    ),
    markerCriterion(filter, "scene_orientation", CriterionModifier.Equals, [
      "Portrait",
    ]),
    markerCriterion(
      filter,
      "scene_performer_tags",
      CriterionModifier.IncludesAll,
      { items: [{ id: "2", label: "Performer tag" }], excluded: [], depth: 0 }
    ),
  ];
  return filter;
}

const expectedOutput = {
  tags: {
    value: ["1"],
    excludes: [],
    modifier: CriterionModifier.Includes,
    depth: 0,
  },
  scene_filter: {
    path: { value: "/clips", modifier: CriterionModifier.Includes },
    performer_favorite: true,
    orientation: { value: ["PORTRAIT"] },
    performer_tags: {
      value: ["2"],
      excludes: [],
      modifier: CriterionModifier.IncludesAll,
      depth: 0,
    },
  },
};

function savedFilter(objectFilter: object): SavedFilterDataFragment {
  return {
    __typename: "SavedFilter",
    id: "1",
    mode: FilterMode.SceneMarkers,
    name: "test",
    find_filter: null,
    object_filter: objectFilter,
    ui_options: null,
  };
}

describe("marker list filters on the marker's scene (#1548)", () => {
  it("offers scene criteria on the marker list", () => {
    const filter = new ListFilterModel(FilterMode.SceneMarkers);
    const types = filter.options.criterionOptions.map((o) => o.type);
    expect(types).toEqual(
      expect.arrayContaining([
        "scene_title",
        "scene_path",
        "scene_studios",
        "scene_performer_tags",
        "scene_performer_count",
        "scene_performer_favorite",
        "scene_orientation",
        "scene_rating",
      ])
    );
  });

  it("sends scene criteria inside scene_filter, using the scene's names", () => {
    expect(makeMarkerFilter().makeFilter()).toEqual(expectedOutput);
  });

  it("keeps scene criteria through the web address", () => {
    const params = makeMarkerFilter().makeQueryParameters();
    const loaded = new ListFilterModel(FilterMode.SceneMarkers);
    loaded.configureFromQueryString(decodeURI(params));
    expect(loaded.makeFilter()).toEqual(expectedOutput);
  });

  it("keeps scene criteria through a saved filter", () => {
    const saved = makeMarkerFilter().makeSavedFilter();
    expect(Object.keys(saved)).toEqual([
      "tags",
      "scene_path",
      "scene_performer_favorite",
      "scene_orientation",
      "scene_performer_tags",
    ]);

    const loaded = new ListFilterModel(FilterMode.SceneMarkers);
    loaded.configureFromSavedFilter(savedFilter(saved));
    expect(loaded.makeFilter()).toEqual(expectedOutput);
  });

  it("loads saved filters made before this change as before", () => {
    const loaded = new ListFilterModel(FilterMode.SceneMarkers);
    loaded.configureFromSavedFilter(
      savedFilter({
        scene_tags: {
          value: { items: [{ id: "3", label: "Scene tag" }], depth: 0 },
          modifier: CriterionModifier.Includes,
        },
        scene_date: {
          value: { value: "2024-01-01" },
          modifier: CriterionModifier.GreaterThan,
        },
        duration: {
          value: { value: 10 },
          modifier: CriterionModifier.LessThan,
        },
      })
    );
    expect(loaded.makeFilter()).toEqual({
      scene_tags: {
        value: ["3"],
        excludes: [],
        modifier: CriterionModifier.Includes,
        depth: 0,
      },
      scene_date: {
        value: "2024-01-01",
        modifier: CriterionModifier.GreaterThan,
      },
      duration: { value: 10, modifier: CriterionModifier.LessThan },
    });
  });
});
