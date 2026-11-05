import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { expect, it, vi } from "vitest";
import { FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { SearchTermInput } from "./ListFilter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

// captures the input's setValue so a test can "type"
let setValue: ((v: string) => void) | undefined;
vi.mock("../Shared/ClearableInput", () => ({
  ClearableInput: (props: { setValue: (v: string) => void }) => {
    setValue = props.setValue;
    return null;
  },
}));

function type(immediate: boolean) {
  vi.useFakeTimers();
  const filter = new ListFilterModel(FilterMode.Scenes);
  filter.currentPage = 3;
  let updated: ListFilterModel | undefined;
  renderToStaticMarkup(
    <IntlProvider locale="en" onError={() => {}}>
      <SearchTermInput
        filter={filter}
        onFilterUpdate={(f) => {
          updated = f;
        }}
        immediate={immediate}
      />
    </IntlProvider>
  );
  setValue!("beach");
  const straightAway = updated;
  vi.advanceTimersByTime(600);
  vi.useRealTimers();
  return { straightAway, later: updated };
}

// #2179: the Edit Filter dialog's Apply must see a term typed just before it
it("sets the term on each key press when immediate", () => {
  const { straightAway } = type(true);
  expect(straightAway?.searchTerm).toBe("beach");
  expect(straightAway?.currentPage).toBe(1);
});

it("waits for typing to pause otherwise, as before", () => {
  const { straightAway, later } = type(false);
  expect(straightAway).toBeUndefined();
  expect(later?.searchTerm).toBe("beach");
});
