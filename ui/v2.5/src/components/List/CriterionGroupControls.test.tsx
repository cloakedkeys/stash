import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { CriterionGroupControls } from "./CriterionGroupControls";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

function render(mode: FilterMode) {
  const filter = new ListFilterModel(mode);
  const criterion = filter.makeCriterion("tags");
  filter.criteria = [criterion];
  return renderToStaticMarkup(
    <IntlProvider locale="en" onError={() => {}}>
      <CriterionGroupControls
        filter={filter}
        criterion={criterion}
        onChange={() => {}}
      />
    </IntlProvider>
  );
}

// groups and "or empty" are sent with AND/OR/NOT/ALL_OF
it("offers groups on lists that can send them", () => {
  expect(render(FilterMode.Scenes)).toContain('id="criterion-group"');
});

it("offers no groups on the Markers list, which can't send them", () => {
  expect(render(FilterMode.SceneMarkers)).toBe("");
});
