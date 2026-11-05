import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { CustomFieldsCriterion } from "src/models/list-filter/criteria/custom-fields";
import { useFilterOperations } from "./util";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

const source = { field: "source", value: ["dvd"], modifier: "EQUALS" };
const region = { field: "region", value: ["2"], modifier: "NOT_EQUALS" };

// runs the list pages' removeCriterion, as the x on a filter tag does
function remove(filter: ListFilterModel, valueIndex?: number) {
  let result = filter;
  const Probe = () => {
    const { removeCriterion } = useFilterOperations({
      filter,
      setFilter: (v) => {
        result = typeof v === "function" ? v(filter) : v;
      },
    });
    removeCriterion(filter.criteria[0], valueIndex);
    return null;
  };
  renderToStaticMarkup(<Probe />);
  return result;
}

function customFieldsFilter() {
  const filter = new ListFilterModel(FilterMode.Scenes);
  const c = filter.makeCriterion("custom_fields");
  c.setFromSavedCriterion([source, region]);
  filter.criteria = [c];
  filter.currentPage = 3;
  return filter;
}

// #2173
it("the x on one custom field value removes just that value", () => {
  const removed = remove(customFieldsFilter(), 0);
  expect(removed.criteria).toHaveLength(1);
  expect((removed.criteria[0] as CustomFieldsCriterion).value).toEqual([
    region,
  ]);
  expect(removed.currentPage).toBe(1);
});

it("the x on a whole condition still removes it", () => {
  expect(remove(customFieldsFilter()).criteria).toHaveLength(0);
});
