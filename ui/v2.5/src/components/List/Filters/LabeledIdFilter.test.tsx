import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { TagsCriterionOption } from "src/models/list-filter/criteria/tags";
import { ModifierCriterion } from "src/models/list-filter/criteria/criterion";
import { ILabeledValueListValue } from "src/models/list-filter/types";
import { useCriterion } from "./LabeledIdFilter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

// The Tags, Studios and Performers sidebar sections show and edit the first
// condition of their kind. Editing it must keep any other condition of the
// same kind (#1375).
it("a sidebar edit keeps the other conditions of its kind", () => {
  const filter = new ListFilterModel(FilterMode.Scenes);
  const includes = filter.makeCriterion("tags");
  includes.setFromSavedCriterion({
    value: { items: [{ id: "1", label: "T1" }], excluded: [], depth: 0 },
    modifier: "INCLUDES",
  });
  const excludes = filter.makeCriterion("tags");
  excludes.setFromSavedCriterion({
    value: { items: [], excluded: [{ id: "2", label: "T2" }], depth: 0 },
    modifier: "INCLUDES",
  });
  filter.criteria = [includes, excludes];

  let hook: ReturnType<typeof useCriterion> | undefined;
  let result: ListFilterModel | undefined;
  const Probe = () => {
    hook = useCriterion(TagsCriterionOption, filter, (f) => {
      result = f;
    });
    return null;
  };
  renderToStaticMarkup(<Probe />);

  // the sidebar shows the first condition
  expect(hook!.criterion).toBe(includes);

  // unselecting T1 leaves "excludes T2"
  const cleared =
    hook!.criterion.clone() as ModifierCriterion<ILabeledValueListValue>;
  cleared.value = { items: [], excluded: [], depth: 0 } as never;
  hook!.setCriterion(cleared);
  expect(result!.makeFilter()).toEqual({
    tags: { value: [], excludes: ["2"], modifier: "INCLUDES", depth: 0 },
  });
});
