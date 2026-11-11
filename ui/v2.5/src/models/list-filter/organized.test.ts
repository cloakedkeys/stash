import { describe, expect, it, vi } from "vitest";
import { CriterionModifier, FilterMode } from "src/core/generated-graphql";
import { getFilterOptions } from "./factory";
import { ListFilterModel } from "./filter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

describe.each([
  FilterMode.Performers,
  FilterMode.Tags,
])("%s organized filter", (mode) => {
  it("offers Organized and sends both yes and no to the API", () => {
    expect(
      getFilterOptions(mode).criterionOptions.some(
        (option) => option.type === "organized"
      )
    ).toBe(true);
    for (const value of [true, false]) {
      const filter = new ListFilterModel(mode);
      const criterion = filter.makeCriterion("organized");
      criterion.setFromSavedCriterion({
        value: String(value),
        modifier: CriterionModifier.Equals,
      });
      filter.criteria = [criterion];
      expect(filter.makeFilter()).toEqual({ organized: value });
    }
  });
});
