import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import {
  faList,
  faSquare,
  faTags,
  faThLarge,
} from "@fortawesome/free-solid-svg-icons";
import { FilterMode } from "src/core/generated-graphql";
import { useCanModify } from "src/hooks/CurrentUser";
import { ListFilterModel } from "src/models/list-filter/filter";
import { DisplayMode } from "src/models/list-filter/types";
import { ListViewButtonGroup } from "./ListViewOptions";

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useLayoutEffect: () => {},
}));
vi.mock("src/hooks/CurrentUser", () => ({ useCanModify: vi.fn() }));
vi.mock("./ZoomSlider", () => ({ ZoomSelect: () => null }));
vi.mock("../Shared/Icon", () => ({
  Icon: ({ icon }: { icon: { iconName: string } }) => (
    <span data-icon={icon.iconName} />
  ),
}));

beforeEach(() => {
  vi.mocked(useCanModify).mockReturnValue(true);
});

const cases = [
  ["Scenes", FilterMode.Scenes, [faThLarge, faList, faSquare, faTags]],
  ["Audio", FilterMode.Audios, [faThLarge, faList]],
  ["Galleries", FilterMode.Galleries, [faThLarge, faList, faSquare]],
  ["Images", FilterMode.Images, [faThLarge, faSquare]],
  ["Groups", FilterMode.Groups, []],
  ["Appears With", FilterMode.Performers, [faThLarge, faList, faTags]],
] as const;

function renderViews(mode: FilterMode) {
  const filter = new ListFilterModel(mode);
  const markup = renderToStaticMarkup(
    <IntlProvider locale="en" onError={() => {}}>
      <ListViewButtonGroup
        displayMode={filter.displayMode}
        displayModeOptions={filter.options.displayModeOptions}
        onSetDisplayMode={() => {}}
      />
    </IntlProvider>
  );
  return { filter, markup };
}

it.each(cases)("puts %s's views in the shared row", (_, mode, icons) => {
  const { markup } = renderViews(mode);
  expect(markup.match(/<button\b/g) ?? []).toHaveLength(icons.length);
  if (icons.length > 1) {
    expect(markup).toContain('class="display-mode-button-group btn-group"');
    expect(markup.match(/data-icon="([^"]+)"/g)).toEqual(
      icons.map((icon) => `data-icon="${icon.iconName}"`)
    );
  } else {
    expect(markup).not.toContain("display-mode-button-group");
  }
});

it("keeps Groups in its sole supported view without a chooser", () => {
  const { filter, markup } = renderViews(FilterMode.Groups);
  expect(filter.displayMode).toBe(DisplayMode.Grid);
  expect(filter.options.displayModeOptions).toEqual([DisplayMode.Grid]);
  expect(markup).not.toContain("<button");
});

it.each([
  FilterMode.Scenes,
  FilterMode.Performers,
])("keeps the view row when the user's role hides Tagger (%s)", (mode) => {
  vi.mocked(useCanModify).mockReturnValue(false);
  const { markup } = renderViews(mode);
  expect(markup).toContain("display-mode-button-group");
  expect(markup).not.toContain(`data-icon="${faTags.iconName}"`);
});
