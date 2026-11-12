import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import messages from "src/locales/en-GB.json";
import * as GQL from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { FilteredSceneList } from "./SceneList";

vi.mock("./SceneCardGrid", () => ({ SceneCardGrid: () => null }));
vi.mock("./SceneListTable", () => ({ SceneListTable: () => null }));
vi.mock("./SceneWallPanel", () => ({ SceneWallPanel: () => null }));
vi.mock("src/components/Tagger/scenes/SceneTagger", () => ({
  Tagger: () => null,
}));
vi.mock("../Groups/GroupDetails/GroupSceneNumbers", () => ({
  GroupSceneNumbers: (props: { children: ReactNode }) => props.children,
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({ error: vi.fn() }) }));
vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({ push: vi.fn() }),
  useLocation: () => ({ search: "" }),
  Link: () => null,
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
}));
vi.mock("src/components/List/ItemList", () => ({
  useFilteredItemList: () => {
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    return {
      filterState: { filter, setFilter: vi.fn() },
      queryResult: {
        effectiveFilter: filter,
        result: {},
        cachedResult: {},
        items: [],
        totalCount: 0,
      },
      listSelect: {
        selectedIds: new Set(["1", "2"]),
        selectedItems: [],
        hasSelection: true,
      },
      modalState: {},
    };
  },
}));
vi.mock("src/components/List/util", () => ({
  useFilterOperations: () => ({}),
  useCloseEditDelete: () => vi.fn(),
}));
vi.mock("src/components/List/keyboardShortcuts", () => ({
  useListOperationShortcuts: () => {},
}));
vi.mock("src/components/List/ZoomSlider", () => ({
  useZoomKeybinds: () => {},
}));
vi.mock("src/components/List/Filters/FilterSidebar", () => ({
  useFilteredSidebarKeybinds: () => {},
  FilteredSidebarHeader: () => null,
}));
vi.mock("src/components/Tagger/context", () => ({
  TaggerContext: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("src/components/Shared/Sidebar", async (original) => ({
  ...(await original<typeof import("src/components/Shared/Sidebar")>()),
  useSidebarState: () => ({}),
  Sidebar: () => null,
  SidebarPane: ({ children }: { children: ReactNode }) => <>{children}</>,
  SidebarPaneContent: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
}));
vi.mock("src/components/List/FilteredListToolbar", () => ({
  FilteredListToolbar: ({
    operationComponent,
  }: {
    operationComponent: ReactNode;
  }) => <>{operationComponent}</>,
}));
vi.mock("src/components/List/ListOperationButtons", () => ({
  ListOperations: ({
    operations,
  }: {
    operations: { text: string; isDisplayed?: () => boolean }[];
  }) => (
    <>
      {operations
        .filter((operation) => operation.isDisplayed?.() ?? true)
        .map((operation) => (
          <button type="button" key={operation.text}>
            {operation.text}
          </button>
        ))}
    </>
  ),
}));
vi.mock("src/components/List/FilterTags", () => ({ FilterTags: () => null }));
vi.mock("src/components/List/Pagination", () => ({
  Pagination: () => null,
  PaginationIndex: () => null,
}));
vi.mock("src/components/List/PagedList", () => ({ LoadedContent: () => null }));

it("offers Merge metadata in the selected scenes actions", () => {
  const html = renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      messages={{ "actions.merge_metadata": messages.actions.merge_metadata }}
      onError={() => {}}
    >
      <FilteredSceneList />
    </IntlProvider>
  );
  expect(html).toContain("Merge metadata…");
  expect(messages.actions.merge).toBe("Merge");
});
