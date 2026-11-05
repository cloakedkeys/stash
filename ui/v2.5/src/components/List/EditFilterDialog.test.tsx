import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { filterToApply, saveEditedFilter } from "./EditFilterDialog";

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

// #2178
it("Save… saves the edited filter, on page 1", async () => {
  const original = filterOnPage3();
  const edited = original.clone();
  edited.criteria = [edited.criteria[0]];

  const save = vi.fn(async () => {});
  const applied = await saveEditedFilter(save, original, edited, "mine", "7");

  expect(save).toHaveBeenCalledTimes(1);
  const [saved, name, id] = save.mock.calls[0] as unknown as [
    ListFilterModel,
    string,
    string,
  ];
  expect([name, id]).toEqual(["mine", "7"]);
  expect(saved.makeSavedFilter()).toEqual(edited.makeSavedFilter());
  expect(saved.currentPage).toBe(1);
  expect(applied).toBe(saved);
});

it("Save… with nothing changed saves the filter as it is", async () => {
  const original = filterOnPage3();
  const save = vi.fn(async () => {});
  await saveEditedFilter(save, original, original.clone(), "mine");
  const saved = (save.mock.calls[0] as unknown as [ListFilterModel])[0];
  expect(saved.makeSavedFilter()).toEqual(original.makeSavedFilter());
  expect(saved.currentPage).toBe(3);
});
