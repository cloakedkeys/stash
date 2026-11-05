import { describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { StringCriterion } from "src/models/list-filter/criteria/criterion";
import { ListFilterModel } from "src/models/list-filter/filter";
import {
  makeSceneFilter,
  sceneFilterFromQuery,
  setSceneFilterQuery,
} from "./sceneFilter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

describe("duplicate checker filter in the page address", () => {
  it("keeps the criteria and the other settings through a reload", () => {
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    const organized = filter.makeCriterion("organized") as StringCriterion;
    organized.value = "false";
    const path = filter.makeCriterion("path") as StringCriterion;
    path.modifier = GQL.CriterionModifier.Excludes;
    path.value = "/media/100% {fake} & co=+#;?";
    filter.criteria.push(organized, path);

    const before = new URLSearchParams("distance=4&c=old&selectedCodec=h264");
    const after = setSceneFilterQuery(before, filter);

    expect(after.get("distance")).toBe("4");
    expect(after.get("selectedCodec")).toBe("h264");
    expect(after.getAll("c")).toHaveLength(2);

    // what the page reads back after history.push and a reload
    const read = sceneFilterFromQuery("?" + after.toString());
    expect(makeSceneFilter(read)).toEqual({
      organized: false,
      path: {
        modifier: GQL.CriterionModifier.Excludes,
        value: "/media/100% {fake} & co=+#;?",
      },
    });
  });

  it("removes the criteria when the filter is cleared", () => {
    const before = new URLSearchParams("distance=4&c=x&c=y");
    const after = setSceneFilterQuery(
      before,
      new ListFilterModel(GQL.FilterMode.Scenes)
    );
    expect(after.toString()).toBe("distance=4");
    expect(makeSceneFilter(sceneFilterFromQuery(after.toString()))).toBe(
      undefined
    );
  });
});
