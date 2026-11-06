import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { PerformersCriterion } from "src/models/list-filter/criteria/performers";
import PerformersFilter from "./PerformersFilter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("src/core/generated-graphql", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useFindPerformersForSelectQuery: () => ({ data: undefined, loading: false }),
}));

const only = "(criterion_modifier_values.only)";

function performersCriterion(mode: FilterMode, saved: unknown) {
  const filter = new ListFilterModel(mode);
  const c = filter.makeCriterion("performers") as PerformersCriterion;
  c.setFromSavedCriterion(saved as never);
  filter.criteria = [c];
  return { filter, c };
}

function render(c: PerformersCriterion) {
  return renderToStaticMarkup(
    <IntlProvider locale="en" messages={{}} onError={() => {}}>
      <PerformersFilter criterion={c} setCriterion={() => {}} />
    </IntlProvider>
  );
}

const twoPerformers = {
  value: {
    items: [
      { id: "1", label: "P1" },
      { id: "2", label: "P2" },
    ],
    excluded: [],
  },
  modifier: "INCLUDES_ALL",
};

// #2235: the groups list's performers filter refuses "equals", so the filter
// doesn't offer "only" there. Other lists still offer it.
describe("performers filter, only", () => {
  it("isn't offered in the groups list", () => {
    const { c } = performersCriterion(FilterMode.Groups, twoPerformers);
    expect(c.modifierCriterionOption().modifierOptions).not.toContain("EQUALS");
    const html = render(c);
    expect(html).not.toContain(only);
    expect(html).toContain("(criterion_modifier_values.any_of)");
  });

  it("is still offered in the scenes list", () => {
    const { c } = performersCriterion(FilterMode.Scenes, twoPerformers);
    expect(render(c)).toContain(only);
  });

  // a saved groups filter that already holds "equals" still opens; the list
  // shows the server's error for it
  it("a saved groups filter with equals still opens", () => {
    const { filter, c } = performersCriterion(FilterMode.Groups, {
      ...twoPerformers,
      modifier: "EQUALS",
    });
    expect(() => render(c)).not.toThrow();
    expect(render(c)).toContain(only);
    expect(filter.makeFilter()).toEqual({
      performers: { value: ["1", "2"], excludes: [], modifier: "EQUALS" },
    });
  });
});
