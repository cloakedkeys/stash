import { describe, expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { setObjectFilter } from "src/components/List/Filters/LabeledIdFilter";
import { getFilterOptions } from "./factory";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

describe("story list filters", () => {
  it("offers only the sorts the story query accepts", () => {
    const sorts = getFilterOptions(FilterMode.Stories).sortByOptions.map(
      (o) => o.value
    );
    expect(sorts).toEqual([
      "title",
      "author",
      "date",
      "rating",
      "created_at",
      "updated_at",
    ]);
  });

  // the tag list in the sidebar is built from the list's filter
  it("lets the tag sidebar build its query for a story list", () => {
    const out = {};
    expect(() =>
      setObjectFilter(out, FilterMode.Stories, { title: undefined })
    ).not.toThrow();
    expect(out).toEqual({});
  });
});
