import { describe, expect, it, vi } from "vitest";
import {
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { ListFilterModel } from "./filter";
import { CriterionType } from "./types";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

// #1206: conditions can be put in Any of / None of groups, several of each.
// Each group is one item of the filter's ALL_OF list. #1170: a condition can
// also match items where its value is empty.

function savedFilter(objectFilter: Record<string, unknown>) {
  return {
    find_filter: {},
    object_filter: objectFilter,
    ui_options: {},
  } as unknown as SavedFilterDataFragment;
}

// [type, saved criterion, group, also matches empty values]
type TestCriterion = [CriterionType, unknown, string?, boolean?];

function sceneFilter(criteria: TestCriterion[]) {
  const filter = new ListFilterModel(FilterMode.Scenes);
  filter.criteria = criteria.map(([type, saved, group, orEmpty]) => {
    const c = filter.makeCriterion(type);
    c.setFromSavedCriterion(saved);
    c.filterGroup = group;
    c.orEmpty = orEmpty;
    return c;
  });
  return filter;
}

function tags(id: string) {
  return {
    value: { items: [{ id, label: `T${id}` }], excluded: [], depth: 0 },
    modifier: "INCLUDES",
  };
}

function performers(id: string) {
  return {
    value: { items: [{ id, label: `P${id}` }], excluded: [] },
    modifier: "INCLUDES",
  };
}

const title = { value: "beach", modifier: "INCLUDES" };
const tagsInput = (id: string) => ({
  value: [id],
  excludes: [],
  modifier: "INCLUDES",
  depth: 0,
});
const performersInput = (id: string) => ({
  value: [id],
  excludes: [],
  modifier: "INCLUDES",
});

// (tag 1 or tag 2) and (performer 3 or performer 4) and not (tag 5 or tag 6)
// and title
function twoAnyOneNone() {
  return sceneFilter([
    ["tags", tags("1"), "any:1"],
    ["performers", performers("3"), "any:2"],
    ["title", title],
    ["tags", tags("2"), "any:1"],
    ["tags", tags("5"), "none:3"],
    ["performers", performers("4"), "any:2"],
    ["tags", tags("6"), "none:3"],
  ]);
}

function summary(f: ListFilterModel) {
  return f.criteria.map((c) => [
    c.criterionOption.type,
    c.toQueryParams(),
    f.groups().find((g) => g.name === c.filterGroup)?.kind,
    !!c.orEmpty,
  ]);
}

describe("Any of and None of groups", () => {
  it("sends each group as an ALL_OF item", () => {
    expect(twoAnyOneNone().makeFilter()).toEqual({
      title,
      ALL_OF: [
        { tags: tagsInput("1"), OR: { tags: tagsInput("2") } },
        {
          performers: performersInput("3"),
          OR: { performers: performersInput("4") },
        },
        { NOT: { tags: tagsInput("5"), OR: { tags: tagsInput("6") } } },
      ],
    });
  });

  it("numbers the groups of each kind in order", () => {
    const groups = twoAnyOneNone().groups();
    expect(groups.map((g) => [g.kind, g.number, g.criteria.length])).toEqual([
      ["any", 1, 2],
      ["any", 2, 2],
      ["none", 1, 2],
    ]);
  });

  it("saves groups with OR and NOT and loads them back", () => {
    const filter = twoAnyOneNone();
    const saved = filter.makeSavedFilter();
    expect(saved).toEqual({
      title,
      ALL_OF: [
        { tags: tags("1"), OR: { tags: tags("2") } },
        { performers: performers("3"), OR: { performers: performers("4") } },
        { NOT: { tags: tags("5"), OR: { tags: tags("6") } } },
      ],
    });

    const loaded = new ListFilterModel(FilterMode.Scenes);
    loaded.configureFromSavedFilter(savedFilter(saved));
    expect(loaded.makeSavedFilter()).toEqual(saved);
    expect(loaded.makeFilter()).toEqual(filter.makeFilter());
    expect(loaded.groups().map((g) => g.kind)).toEqual(["any", "any", "none"]);
  });

  it("keeps groups in the page address, one c= value each", () => {
    const filter = twoAnyOneNone();
    const query = filter.makeQueryParameters();
    expect(query.match(/c=/g)).toHaveLength(4);
    const decoded = decodeURIComponent(query);
    expect(decoded).toContain('c=("type":"_any","c":[("type":"tags"');
    expect(decoded).toContain('c=("type":"_none","c":[');

    const fromAddress = new ListFilterModel(FilterMode.Scenes);
    fromAddress.configureFromQueryString(query);
    expect(fromAddress.makeFilter()).toEqual(filter.makeFilter());
    expect(fromAddress.makeQueryParameters()).toBe(query);
  });

  it("puts a None of group with one condition under NOT", () => {
    const filter = sceneFilter([["tags", tags("5"), "none:1"]]);
    expect(filter.makeFilter()).toEqual({
      ALL_OF: [{ NOT: { tags: tagsInput("5") } }],
    });
  });

  it("works with repeated conditions outside groups", () => {
    const filter = sceneFilter([
      ["tags", tags("1")],
      ["tags", tags("2")],
      ["tags", tags("3"), "any:1"],
      ["tags", tags("4"), "any:1"],
    ]);
    expect(filter.makeFilter()).toEqual({
      tags: tagsInput("1"),
      AND: { tags: tagsInput("2") },
      ALL_OF: [{ tags: tagsInput("3"), OR: { tags: tagsInput("4") } }],
    });
  });

  it("gives a new group a name not in use", () => {
    const filter = twoAnyOneNone();
    expect(filter.newGroupName("any")).toBe("any:4");
    expect(filter.newGroupName("none")).toBe("none:4");
  });
});

// #1170
describe("conditions that also match empty values", () => {
  const rating = {
    value: { value: 20, value2: undefined },
    modifier: "NOT_EQUALS",
  };

  it("send the condition or its is-empty form", () => {
    const filter = sceneFilter([
      ["title", title],
      ["rating100", rating, undefined, true],
    ]);
    expect(filter.makeFilter()).toEqual({
      title,
      ALL_OF: [
        {
          rating100: { value: 20, modifier: "NOT_EQUALS" },
          OR: { rating100: expect.objectContaining({ modifier: "IS_NULL" }) },
        },
      ],
    });
  });

  it("add the is-empty form inside an Any of group", () => {
    const filter = sceneFilter([
      ["rating100", rating, "any:1", true],
      ["tags", tags("1"), "any:1"],
    ]);
    expect(filter.makeFilter()).toEqual({
      ALL_OF: [
        {
          rating100: { value: 20, modifier: "NOT_EQUALS" },
          OR: {
            rating100: expect.objectContaining({ modifier: "IS_NULL" }),
            OR: { tags: tagsInput("1") },
          },
        },
      ],
    });
  });

  it("save and load back as the same condition", () => {
    const filter = sceneFilter([
      ["title", title],
      ["rating100", rating, undefined, true],
      ["tags", tags("1"), "none:1"],
      ["rating100", rating, "none:1", true],
    ]);

    const loaded = new ListFilterModel(FilterMode.Scenes);
    loaded.configureFromSavedFilter(savedFilter(filter.makeSavedFilter()));
    expect(summary(loaded)).toEqual(summary(filter));
    expect(loaded.makeFilter()).toEqual(filter.makeFilter());

    const fromAddress = new ListFilterModel(FilterMode.Scenes);
    fromAddress.configureFromQueryString(filter.makeQueryParameters());
    expect(summary(fromAddress)).toEqual(summary(filter));
  });

  it("are sent as before when not set or not possible", () => {
    const filter = sceneFilter([
      ["title", title, undefined, false],
      // already "is empty"
      ["date", { value: {}, modifier: "IS_NULL" }, undefined, true],
    ]);
    const output = filter.makeFilter();
    expect(output).not.toHaveProperty("ALL_OF");
    expect(output).toHaveProperty("title", title);
  });
});

// the sidebar filters show and edit conditions outside groups only
describe("sidebar edits leave groups alone", () => {
  it("replace and remove only conditions outside groups", () => {
    const filter = sceneFilter([
      ["tags", tags("1"), "any:1"],
      ["tags", tags("2"), "any:1"],
      ["tags", tags("3")],
    ]);
    expect(filter.criteriaFor("tags")).toEqual([filter.criteria[2]]);

    const edited = filter.makeCriterion("tags");
    edited.setFromSavedCriterion(tags("9"));
    expect(filter.replaceCriteria("tags", [edited]).makeFilter()).toEqual({
      tags: tagsInput("9"),
      ALL_OF: [{ tags: tagsInput("1"), OR: { tags: tagsInput("2") } }],
    });

    for (const removed of [
      filter.replaceCriteria("tags", []),
      filter.removeCriterion("tags"),
    ]) {
      expect(removed.makeFilter()).toEqual({
        ALL_OF: [{ tags: tagsInput("1"), OR: { tags: tagsInput("2") } }],
      });
    }
  });

  it("find nothing to edit when all conditions of a kind are grouped", () => {
    const filter = sceneFilter([["tags", tags("1"), "any:1"]]);
    expect(filter.criteriaFor("tags")).toEqual([]);
    expect(filter.removeCriterion("tags").criteria).toHaveLength(1);
  });
});

describe("removing one value of a custom fields condition", () => {
  const source = { field: "source", value: ["dvd"], modifier: "EQUALS" };
  const region = { field: "region", value: ["2"], modifier: "NOT_EQUALS" };
  const studio = { field: "studio", value: ["x"], modifier: "EQUALS" };

  it("changes the condition it belongs to, even in a group", () => {
    const filter = sceneFilter([
      ["custom_fields", [source]],
      ["custom_fields", [region, studio], "any:1"],
      ["tags", tags("1"), "any:1"],
    ]);

    const removed = filter.removeCustomFieldValue(filter.criteria[1], 0);
    expect(removed.makeFilter()).toEqual({
      custom_fields: [source],
      ALL_OF: [{ custom_fields: [studio], OR: { tags: tagsInput("1") } }],
    });
    // the filter it came from is unchanged
    expect(filter.makeFilter()).toHaveProperty("custom_fields", [source]);
  });
});

// SceneMarkerFilterType has no AND, OR, NOT or ALL_OF
describe("groups on a list that can't send them", () => {
  function markerFilter(criteria: TestCriterion[]) {
    const filter = new ListFilterModel(FilterMode.SceneMarkers);
    filter.criteria = criteria.map(([type, saved, group, orEmpty]) => {
      const c = filter.makeCriterion(type);
      c.setFromSavedCriterion(saved);
      c.filterGroup = group;
      c.orEmpty = orEmpty;
      return c;
    });
    return filter;
  }

  it("leave grouped conditions out and send the rest plainly", () => {
    const filter = markerFilter([
      ["tags", tags("1"), undefined, true],
      ["tags", tags("2"), "any:1"],
      ["tags", tags("3"), "none:2"],
    ]);
    expect(filter.makeFilter()).toEqual({ tags: tagsInput("1") });
  });

  it("are still saved, as on other lists", () => {
    const filter = markerFilter([
      ["tags", tags("1")],
      ["tags", tags("3"), "none:1"],
    ]);
    expect(filter.makeSavedFilter()).toEqual({
      tags: tags("1"),
      ALL_OF: [{ NOT: { tags: tags("3") } }],
    });
  });
});

// the server returns saved filter keys in its own order
it("loads a saved group whose OR comes before its condition", () => {
  const loaded = new ListFilterModel(FilterMode.Scenes);
  loaded.configureFromSavedFilter(
    savedFilter({
      ALL_OF: [{ OR: { tags: tags("2") }, tags: tags("1") }],
    })
  );
  expect(loaded.makeFilter()).toEqual({
    ALL_OF: [{ tags: tagsInput("1"), OR: { tags: tagsInput("2") } }],
  });
});
