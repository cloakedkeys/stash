import React from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { FilterMode, SortDirectionEnum } from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import { GroupSceneNumbers } from "../Groups/GroupDetails/GroupSceneNumbers";
import { FilteredSceneList } from "./SceneList";

const page = vi.hoisted(() => ({
  filter: undefined as ListFilterModel | undefined,
  canModify: true,
  refetch: vi.fn(),
  move: vi.fn(),
  error: vi.fn(),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useMemo: (fn: () => unknown) => fn(),
  useCallback: (fn: unknown) => fn,
  useEffect: () => {},
}));
vi.mock("react-intl", async (original) => ({
  ...(await original<typeof import("react-intl")>()),
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("react-router-dom", async (original) => ({
  ...(await original<typeof import("react-router-dom")>()),
  useHistory: () => ({}),
  useLocation: () => ({ search: "" }),
}));
vi.mock("../Shared/HoverScrubber", () => ({ HoverScrubber: () => null }));
vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanModify: () => page.canModify,
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({ error: page.error }) }));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
}));
vi.mock("src/utils/focus", () => ({ default: () => [] }));
vi.mock("../Shared/Sidebar", async (original) => ({
  ...(await original<typeof import("../Shared/Sidebar")>()),
  useSidebarState: () => ({}),
}));
vi.mock("../List/ItemList", () => ({
  useFilteredItemList: () => ({
    filterState: { filter: page.filter },
    queryResult: {
      effectiveFilter: page.filter,
      result: { refetch: page.refetch },
      cachedResult: {},
      items: [],
      totalCount: 0,
    },
    modalState: {},
    listSelect: { selectedIds: new Set(), selectedItems: [] },
  }),
}));
vi.mock("../List/util", async (original) => ({
  ...(await original<typeof import("../List/util")>()),
  useFilterOperations: () => ({}),
  useCloseEditDelete: () => vi.fn(),
}));
vi.mock("../List/Filters/FilterSidebar", async (original) => ({
  ...(await original<typeof import("../List/Filters/FilterSidebar")>()),
  useFilteredSidebarKeybinds: () => {},
}));
vi.mock("../List/ZoomSlider", async (original) => ({
  ...(await original<typeof import("../List/ZoomSlider")>()),
  useZoomKeybinds: () => {},
}));
vi.mock("../List/keyboardShortcuts", async (original) => ({
  ...(await original<typeof import("../List/keyboardShortcuts")>()),
  useListOperationShortcuts: () => {},
}));

type Props = {
  children?: React.ReactNode;
  scenes?: unknown[];
  onSort?: unknown;
  groupId?: string;
  onSaved?: () => Promise<unknown>;
  onMove?: (ids: string[], target: string, after: boolean) => Promise<void>;
};
function elements(node: React.ReactNode): React.ReactElement<Props>[] {
  return React.Children.toArray(node).flatMap((child) => {
    if (!React.isValidElement<Props>(child)) return [];
    return [child, ...elements(child.props.children)];
  });
}
function render() {
  const tree = elements(
    FilteredSceneList({
      fromGroupId: "10",
      manualSortBy: "group_scene_number",
      onMove: page.move,
    }) as React.ReactNode
  );
  return {
    numbers: tree.find((element) => element.type === GroupSceneNumbers)!.props,
    list: tree.find((element) => element.props.onSort)!.props,
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  page.filter = new ListFilterModel(FilterMode.Scenes);
  page.filter.sortBy = "group_scene_number";
  page.canModify = true;
});
it("offers drag only with modify access and scene-number sorting; manual saves refresh", async () => {
  const allowed = render();
  expect(allowed.list.onMove).toBeTypeOf("function");
  expect(allowed.numbers.groupId).toBe("10");
  await allowed.numbers.onSaved!();
  expect(page.refetch).toHaveBeenCalledOnce();
  page.canModify = false;
  expect(render().list.onMove).toBeUndefined();
  page.canModify = true;
  page.filter!.sortBy = "title";
  expect(render().list.onMove).toBeUndefined();
  expect(render().numbers.groupId).toBe("10");
});
it("refreshes after a successful drop and shows errors when saving fails", async () => {
  page.filter!.sortDirection = SortDirectionEnum.Desc;
  await render().list.onMove!(["1"], "2", true);
  expect(page.move).toHaveBeenCalledExactlyOnceWith(
    ["1"],
    "2",
    true,
    SortDirectionEnum.Desc
  );
  expect(page.refetch).toHaveBeenCalledOnce();
  page.refetch.mockClear();
  const error = new Error("Saving failed");
  page.move.mockRejectedValueOnce(error);
  await render().list.onMove!(["1"], "2", false);
  expect(page.error).toHaveBeenCalledWith(error);
  expect(page.refetch).not.toHaveBeenCalled();
});
