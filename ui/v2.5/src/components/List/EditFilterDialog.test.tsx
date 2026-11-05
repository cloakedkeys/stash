import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { filterToApply } from "./EditFilterDialog";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

function filterOnPage3() {
  const filter = new ListFilterModel(FilterMode.Scenes);
  const title = filter.makeCriterion("title");
  title.setFromSavedCriterion({ value: "beach", modifier: "INCLUDES" });
  const organized = filter.makeCriterion("organized");
  organized.setFromSavedCriterion({ value: "true", modifier: "EQUALS" });
  filter.criteria = [title, organized];
  filter.currentPage = 3;
  return filter;
}

// #2175
it("goes to page 1 when Apply removed a condition", () => {
  const original = filterOnPage3();
  const edited = original.clone();
  edited.criteria = [edited.criteria[0]];
  const applied = filterToApply(original, edited);
  expect(applied.currentPage).toBe(1);
  expect(applied.criteria).toHaveLength(1);
});

it("goes to page 1 when Apply changed the search term", () => {
  const original = filterOnPage3();
  const edited = original.clone();
  edited.searchTerm = "sun";
  expect(filterToApply(original, edited).currentPage).toBe(1);
});

it("stays on the page when Apply changed nothing", () => {
  const original = filterOnPage3();
  expect(filterToApply(original, original.clone()).currentPage).toBe(3);
});
