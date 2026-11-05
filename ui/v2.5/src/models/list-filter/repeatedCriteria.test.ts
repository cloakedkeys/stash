import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";
import {
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { ListFilterModel, supportsSubFilters } from "./filter";
import { CriterionType } from "./types";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

// #821, #1375, #1243: the same kind of condition can be used more than once.
// A repeat goes into an AND sub-filter, in what is sent to the server and in
// saved filters. Filters without a repeat must save and encode exactly as they
// did before.

function savedFilter(objectFilter: Record<string, unknown>) {
  return {
    find_filter: {},
    object_filter: objectFilter,
    ui_options: {},
  } as unknown as SavedFilterDataFragment;
}

function sceneFilter(criteria: [CriterionType, unknown][]) {
  const filter = new ListFilterModel(FilterMode.Scenes);
  filter.criteria = criteria.map(([type, saved]) => {
    const c = filter.makeCriterion(type);
    c.setFromSavedCriterion(saved);
    return c;
  });
  return filter;
}

// what makeFilter, makeSavedFilter and configureFromSavedFilter did before
// repeats were allowed
function legacyMakeFilter(filter: ListFilterModel) {
  const output: Record<string, unknown> = {};
  for (const c of filter.criteria) c.applyToCriterionInput(output);
  return output;
}

function legacyMakeSavedFilter(filter: ListFilterModel) {
  const output: Record<string, unknown> = {};
  for (const c of filter.criteria) c.applyToSavedCriterion(output);
  return output;
}

function legacyLoad(objectFilter: Record<string, unknown>) {
  const filter = new ListFilterModel(FilterMode.Scenes);
  for (const [k, v] of Object.entries(objectFilter)) {
    const c = filter.makeCriterion(k as CriterionType);
    c.setFromSavedCriterion(v);
    filter.criteria.push(c);
  }
  return filter;
}

const tagsIncludesT1 = {
  value: { items: [{ id: "1", label: "T1" }], excluded: [], depth: 0 },
  modifier: "INCLUDES",
};
const tagsIncludesT2 = {
  value: { items: [{ id: "2", label: "T2" }], excluded: [], depth: 0 },
  modifier: "INCLUDES",
};

const missingDate = { value: "date", modifier: "EQUALS" };
const missingTags = { value: "tags", modifier: "EQUALS" };
const missingRating = { value: "rating", modifier: "EQUALS" };

// one of each kind, as saved filters, default filters and front page filters
// hold them
const oneOfEach: Record<string, unknown> = {
  title: { value: "beach", modifier: "INCLUDES" },
  rating100: {
    value: { value: 60, value2: undefined },
    modifier: "GREATER_THAN",
  },
  tags: {
    value: {
      items: [{ id: "12", label: "Outdoor" }],
      excluded: [{ id: "3", label: "Indoor" }],
      depth: 0,
    },
    modifier: "INCLUDES_ALL",
  },
  studios: {
    value: { items: [], excluded: [], depth: 0 },
    modifier: "IS_NULL",
  },
  is_missing: missingDate,
  organized: { value: "true", modifier: "EQUALS" },
  duration: { value: { value: 600, value2: 1200 }, modifier: "BETWEEN" },
  date: {
    value: { value: "2020-01-01", value2: undefined },
    modifier: "GREATER_THAN",
  },
  custom_fields: [
    { field: "source", value: ["dvd"], modifier: "EQUALS" },
    { field: "region", value: ["2"], modifier: "NOT_EQUALS" },
  ],
};

describe("the same kind of condition more than once", () => {
  it("sends a repeat to the server in an AND sub-filter", () => {
    const filter = sceneFilter([
      ["is_missing", missingDate],
      ["is_missing", missingTags],
    ]);

    expect(filter.makeFilter()).toEqual({
      is_missing: "date",
      AND: { is_missing: "tags" },
    });
  });

  it("nests one AND sub-filter for each further repeat", () => {
    const filter = sceneFilter([
      ["is_missing", missingDate],
      ["title", { value: "beach", modifier: "INCLUDES" }],
      ["is_missing", missingTags],
      ["is_missing", missingRating],
    ]);

    expect(filter.makeFilter()).toEqual({
      is_missing: "date",
      title: { value: "beach", modifier: "INCLUDES" },
      AND: { is_missing: "tags", AND: { is_missing: "rating" } },
    });
  });

  // #1375: "studio excludes X" and "studio is empty"
  it("keeps both studio conditions", () => {
    const filter = sceneFilter([
      [
        "studios",
        {
          value: { items: [], excluded: [{ id: "5", label: "X" }], depth: 0 },
          modifier: "INCLUDES",
        },
      ],
      [
        "studios",
        { value: { items: [], excluded: [], depth: 0 }, modifier: "IS_NULL" },
      ],
    ]);

    expect(filter.makeFilter()).toMatchObject({
      studios: { excludes: ["5"] },
      AND: { studios: { modifier: "IS_NULL" } },
    });
  });

  it("saves a repeat in an AND sub-filter and loads it back in order", () => {
    const filter = sceneFilter([
      ["is_missing", missingDate],
      ["title", { value: "beach", modifier: "INCLUDES" }],
      ["is_missing", missingTags],
      ["is_missing", missingRating],
    ]);

    const saved = filter.makeSavedFilter();
    expect(saved).toEqual({
      is_missing: missingDate,
      title: { value: "beach", modifier: "INCLUDES" },
      AND: { is_missing: missingTags, AND: { is_missing: missingRating } },
    });

    const loaded = new ListFilterModel(FilterMode.Scenes);
    loaded.configureFromSavedFilter(savedFilter(saved));
    expect(
      loaded.criteria.map((c) => [c.criterionOption.type, c.toQueryParams()])
    ).toEqual(
      filter.criteria.map((c) => [c.criterionOption.type, c.toQueryParams()])
    );
    expect(loaded.makeSavedFilter()).toEqual(saved);
    expect(loaded.makeFilter()).toEqual(filter.makeFilter());
  });

  // the server returns a saved filter's keys in alphabetical order
  it("loads a saved repeat whatever the order of its keys", () => {
    const loaded = new ListFilterModel(FilterMode.Scenes);
    loaded.configureFromSavedFilter(
      savedFilter({ AND: { is_missing: missingTags }, is_missing: missingDate })
    );
    expect(loaded.criteria).toHaveLength(2);
    expect(loaded.makeFilter()).toEqual({
      is_missing: "tags",
      AND: { is_missing: "date" },
    });
  });

  it("keeps both conditions from a page address that has the kind twice", () => {
    const filter = sceneFilter([
      ["is_missing", missingDate],
      ["is_missing", missingTags],
    ]);
    const query = filter.makeQueryParameters();
    expect(query.match(/c=/g)).toHaveLength(2);

    const fromAddress = new ListFilterModel(FilterMode.Scenes);
    fromAddress.configureFromQueryString(query);
    expect(fromAddress.criteria).toHaveLength(2);
    expect(fromAddress.makeFilter()).toEqual({
      is_missing: "date",
      AND: { is_missing: "tags" },
    });
    expect(fromAddress.makeQueryParameters()).toBe(query);
  });

  it("removes only the condition asked for", () => {
    const filter = sceneFilter([
      ["is_missing", missingDate],
      ["is_missing", missingTags],
      ["is_missing", missingRating],
    ]);

    const removed = filter.removeCriterionInstance(filter.criteria[1]);
    expect(removed.makeFilter()).toEqual({
      is_missing: "date",
      AND: { is_missing: "rating" },
    });
    // the filter it came from is unchanged
    expect(filter.criteria).toHaveLength(3);
  });
});

// the sidebar filters show and edit the first condition of their kind
describe("sidebar edits of a repeated kind", () => {
  const tagsIncludes = {
    value: { items: [{ id: "1", label: "T1" }], excluded: [], depth: 0 },
    modifier: "INCLUDES",
  };
  const tagsExcludes = {
    value: { items: [], excluded: [{ id: "2", label: "T2" }], depth: 0 },
    modifier: "INCLUDES",
  };

  function types(f: ListFilterModel) {
    return f.criteria.map((c) => c.criterionOption.type);
  }

  it("replace only the first condition of the kind and keep the others", () => {
    const filter = sceneFilter([
      ["tags", tagsIncludes],
      ["title", { value: "beach", modifier: "INCLUDES" }],
      ["tags", tagsExcludes],
    ]);
    const edited = filter.makeCriterion("tags");
    edited.setFromSavedCriterion({
      value: { items: [{ id: "3", label: "T3" }], excluded: [], depth: 0 },
      modifier: "INCLUDES",
    });

    const replaced = filter.replaceCriteria("tags", [edited]);
    expect(types(replaced)).toEqual(["tags", "title", "tags"]);
    expect(replaced.makeFilter()).toMatchObject({
      tags: { value: ["3"], modifier: "INCLUDES" },
      AND: { tags: { excludes: ["2"] } },
    });
  });

  it("remove only the first condition of the kind", () => {
    const filter = sceneFilter([
      ["tags", tagsIncludes],
      ["tags", tagsExcludes],
    ]);

    for (const removed of [
      filter.replaceCriteria("tags", []),
      filter.removeCriterion("tags"),
    ]) {
      expect(removed.makeFilter()).toEqual({
        tags: { value: [], excludes: ["2"], modifier: "INCLUDES", depth: 0 },
      });
    }
  });

  it("move a single condition to the end, as before", () => {
    const filter = sceneFilter([
      ["tags", tagsIncludes],
      ["title", { value: "beach", modifier: "INCLUDES" }],
    ]);
    const edited = filter.criteria[0].clone();

    expect(types(filter.replaceCriteria("tags", [edited]))).toEqual([
      "title",
      "tags",
    ]);
    expect(types(filter.replaceCriteria("tags", []))).toEqual(["title"]);
  });
});

// the server filter type of each list mode
const modeFilterTypes: Record<FilterMode, string> = {
  [FilterMode.Audios]: "AudioFilterType",
  [FilterMode.Galleries]: "GalleryFilterType",
  [FilterMode.Groups]: "GroupFilterType",
  [FilterMode.Images]: "ImageFilterType",
  [FilterMode.Movies]: "MovieFilterType",
  [FilterMode.Performers]: "PerformerFilterType",
  [FilterMode.Scenes]: "SceneFilterType",
  [FilterMode.SceneMarkers]: "SceneMarkerFilterType",
  [FilterMode.Stories]: "StoryFilterType",
  [FilterMode.Studios]: "StudioFilterType",
  [FilterMode.Tags]: "TagFilterType",
};

describe("lists whose filter type has no AND", () => {
  it("are known for every list mode, as the GraphQL schema says", () => {
    const schema = readFileSync(
      new URL(
        "../../../../../graphql/schema/types/filters.graphql",
        import.meta.url
      ),
      "utf8"
    );

    for (const mode of Object.values(FilterMode)) {
      const typeName = modeFilterTypes[mode];
      const body = schema.match(
        new RegExp(`^input ${typeName} \\{([\\s\\S]*?)^\\}`, "m")
      )?.[1];
      expect(body, typeName).toBeDefined();
      expect(supportsSubFilters(mode), typeName).toBe(/^\s*AND:/m.test(body!));
    }
    expect(supportsSubFilters(FilterMode.SceneMarkers)).toBe(false);
  });

  it("send the last condition of a repeated kind, as before", () => {
    const filter = new ListFilterModel(FilterMode.SceneMarkers);
    filter.criteria = [
      ["tags", tagsIncludesT1],
      ["tags", tagsIncludesT2],
    ].map(([type, saved]) => {
      const c = filter.makeCriterion(type as CriterionType);
      c.setFromSavedCriterion(saved);
      return c;
    });

    expect(filter.makeFilter()).toEqual(legacyMakeFilter(filter));
    expect(JSON.stringify(filter.makeFilter())).not.toContain('"AND"');
    expect(filter.makeFilter()).toEqual({
      tags: { value: ["2"], excludes: [], modifier: "INCLUDES", depth: 0 },
    });

    // a saved filter keeps both, for the UI to load back
    const loaded = new ListFilterModel(FilterMode.SceneMarkers);
    loaded.configureFromSavedFilter(savedFilter(filter.makeSavedFilter()));
    expect(loaded.criteria).toHaveLength(2);
  });
});

describe("filters without a repeat are unchanged", () => {
  it("loads an old saved filter as before", () => {
    const loaded = new ListFilterModel(FilterMode.Scenes);
    loaded.configureFromSavedFilter(savedFilter(oneOfEach));
    const legacy = legacyLoad(oneOfEach);

    expect(
      loaded.criteria.map((c) => [c.criterionOption.type, c.toQueryParams()])
    ).toEqual(
      legacy.criteria.map((c) => [c.criterionOption.type, c.toQueryParams()])
    );
  });

  it("saves, sends and encodes byte-for-byte as before", () => {
    const filter = new ListFilterModel(FilterMode.Scenes);
    filter.configureFromSavedFilter(savedFilter(oneOfEach));
    expect(filter.criteria).toHaveLength(Object.keys(oneOfEach).length);

    expect(JSON.stringify(filter.makeSavedFilter())).toBe(
      JSON.stringify(legacyMakeSavedFilter(filter))
    );
    expect(JSON.stringify(filter.makeFilter())).toBe(
      JSON.stringify(legacyMakeFilter(filter))
    );
    expect(JSON.stringify(filter.makeFilter())).not.toContain('"AND"');

    // a bookmarked address loads and encodes back to itself
    const query = filter.makeQueryParameters();
    const fromAddress = new ListFilterModel(FilterMode.Scenes);
    fromAddress.configureFromQueryString(query);
    expect(fromAddress.makeQueryParameters()).toBe(query);
    expect(JSON.stringify(fromAddress.makeSavedFilter())).toBe(
      JSON.stringify(legacyMakeSavedFilter(fromAddress))
    );
  });

  it("loads an empty saved filter as before", () => {
    const filter = new ListFilterModel(FilterMode.Scenes);
    filter.configureFromSavedFilter(savedFilter({}));
    expect(filter.criteria).toHaveLength(0);
    expect(filter.makeFilter()).toEqual({});
    expect(filter.makeSavedFilter()).toEqual({});
  });
});
