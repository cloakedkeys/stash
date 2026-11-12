import type { ComponentProps, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import messages from "src/locales/en-GB.json";
import { ListFilterModel } from "src/models/list-filter/filter";
import type { IAlertModalProps } from "src/components/Shared/Alert";
import { GroupScenesPanel } from "../Groups/GroupDetails/GroupScenesPanel";
import { FilteredSceneList } from "./SceneList";

const page = vi.hoisted(() => ({
  update: vi.fn(),
  refetch: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  canModify: true,
  selected: new Set(["scene-1", "scene-3", "scene-5"]),
  modal: undefined as ReactNode,
  buttons: [] as { title?: string; onClick: () => void }[],
}));
vi.mock("src/core/StashService", () => ({
  useBulkSceneUpdate: () => [page.update],
  useScenesUpdate: () => [vi.fn()],
  useFindScenes: vi.fn(),
  queryFindScenes: vi.fn(),
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: page.success, error: page.error }),
}));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanModify: () => page.canModify,
  useCanAdminister: () => false,
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
}));
vi.mock("src/hooks/modal", () => ({
  useModal: () => ({
    modal: null,
    showModal: (m: ReactNode) => {
      page.modal = m;
    },
    closeModal: () => {
      page.modal = undefined;
    },
  }),
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({}),
  useLocation: () => ({ search: "" }),
}));
vi.mock("src/components/List/ItemList", async (original) => ({
  ...(await original<typeof import("src/components/List/ItemList")>()),
  useFilteredItemList: () => {
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    const result = { refetch: page.refetch, loading: false };
    return {
      filterState: { filter, setFilter: vi.fn() },
      queryResult: {
        effectiveFilter: filter,
        result,
        cachedResult: result,
        items: [],
        totalCount: 5,
      },
      listSelect: {
        selectedIds: page.selected,
        selectedItems: [],
        hasSelection: page.selected.size > 0,
      },
      modalState: { modal: null },
    };
  },
}));
vi.mock("src/components/List/FilteredListToolbar", () => ({
  FilteredListToolbar: ({
    operationComponent,
  }: {
    operationComponent: ReactNode;
  }) => operationComponent,
}));
vi.mock("src/components/Shared/Sidebar", async () => {
  const { createContext } = await import("react");
  const children = ({ children }: { children: ReactNode }) => children;
  return {
    SidebarStateContext: createContext({}),
    useSidebarState: () => ({}),
    Sidebar: () => null,
    SidebarPane: children,
    SidebarPaneContent: children,
  };
});
vi.mock("src/components/List/Filters/FilterSidebar", () => ({
  useFilteredSidebarKeybinds: vi.fn(),
}));
vi.mock("src/components/List/ZoomSlider", () => ({ useZoomKeybinds: vi.fn() }));
vi.mock("src/components/List/keyboardShortcuts", () => ({
  useListOperationShortcuts: vi.fn(),
}));
vi.mock("src/components/Tagger/context", () => ({
  TaggerContext: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("src/components/Groups/GroupDetails/GroupSceneNumbers", () => ({
  GroupSceneNumbers: () => null,
}));
vi.mock("src/components/List/FilterTags", () => ({ FilterTags: () => null }));
vi.mock("src/components/List/Pagination", () => ({
  Pagination: () => null,
  PaginationIndex: () => null,
}));
vi.mock("src/components/Scenes/SceneWallPanel", () => ({
  SceneWallPanel: () => null,
}));
vi.mock("src/components/Scenes/SceneCardGrid", () => ({
  SceneCardGrid: () => null,
}));
vi.mock("src/components/Scenes/SceneListTable", () => ({
  SceneListTable: () => null,
}));
vi.mock("src/components/Tagger/scenes/SceneTagger", () => ({
  Tagger: () => null,
}));
vi.mock("src/components/Scenes/EditScenesDialog", () => ({
  EditScenesDialog: () => null,
}));
vi.mock("src/components/Scenes/DeleteScenesDialog", () => ({
  DeleteScenesDialog: () => null,
}));
vi.mock("src/components/Scenes/SceneMergeDialog", () => ({
  SceneMergeModal: () => null,
}));
vi.mock("src/components/Dialogs/GenerateDialog", () => ({
  GenerateDialog: () => null,
}));
vi.mock("src/components/Dialogs/IdentifyDialog/IdentifyDialog", () => ({
  IdentifyDialog: () => null,
}));
vi.mock("src/components/Shared/Icon", () => ({ Icon: () => null }));
vi.mock("react-bootstrap", async (original) => ({
  ...(await original<typeof import("react-bootstrap")>()),
  Button: (props: ComponentProps<"button">) => {
    page.buttons.push(props as (typeof page.buttons)[number]);
    return <button type="button" title={props.title} />;
  },
}));

function render(groupPage = true) {
  page.buttons = [];
  return renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      onError={() => {}}
      messages={{
        ...Object.fromEntries(
          Object.entries(messages.actions).map(([k, v]) => [`actions.${k}`, v])
        ),
        "dialogs.remove_scenes_from_group":
          messages.dialogs.remove_scenes_from_group,
        "toast.removed_entity": messages.toast.removed_entity,
        scene: messages.scene,
        scenes: messages.scenes,
      }}
    >
      {groupPage ? (
        <GroupScenesPanel
          active
          group={{ id: "group-10", name: "Group" } as GQL.GroupDataFragment}
        />
      ) : (
        <FilteredSceneList />
      )}
    </IntlProvider>
  );
}
function removeButton() {
  return page.buttons.find((b) => b?.title === "Remove from group");
}
async function open() {
  render();
  expect(removeButton()).toBeDefined();
  await removeButton()!.onClick();
  return (page.modal as React.ReactElement<IAlertModalProps>).props;
}
beforeEach(() => {
  vi.stubGlobal("document", { body: undefined });
  vi.clearAllMocks();
  page.modal = undefined;
  page.canModify = true;
  page.selected = new Set(["scene-1", "scene-3", "scene-5"]);
  page.update.mockResolvedValue({});
  page.refetch.mockResolvedValue({});
});
it("confirms the selected count, removes only this group for exactly those scenes, then toasts and refreshes", async () => {
  const dialog = await open();
  expect(dialog.text).toBe(
    "Remove 3 scenes from this group? The scenes are kept."
  );
  expect(page.update).not.toHaveBeenCalled();
  page.selected.add("later-selection");
  await dialog.onConfirm();
  expect(page.update).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: {
        ids: ["scene-1", "scene-3", "scene-5"],
        group_ids: { ids: ["group-10"], mode: GQL.BulkUpdateIdMode.Remove },
      },
    },
  });
  expect(page.success).toHaveBeenCalledExactlyOnceWith("Removed Scenes");
  expect(page.refetch).toHaveBeenCalledOnce();
  expect(page.error).not.toHaveBeenCalled();
});
it("cancels without sending a mutation or refreshing", async () => {
  const dialog = await open();
  dialog.onCancel();
  expect(page.modal).toBeUndefined();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.refetch).not.toHaveBeenCalled();
  expect(page.success).not.toHaveBeenCalled();
});
it("offers removal only on a group page, to modifiers, with selected scenes", () => {
  render();
  expect(removeButton()).toBeDefined();
  render(false);
  expect(removeButton()).toBeUndefined();
  page.canModify = false;
  render();
  expect(removeButton()).toBeUndefined();
  page.canModify = true;
  page.selected.clear();
  render();
  expect(removeButton()).toBeUndefined();
});
it("reports a failed update without success or refresh", async () => {
  const dialog = await open();
  const error = new Error("Update failed");
  page.update.mockRejectedValue(error);
  await dialog.onConfirm();
  expect(page.error).toHaveBeenCalledExactlyOnceWith(error);
  expect(page.success).not.toHaveBeenCalled();
  expect(page.refetch).not.toHaveBeenCalled();
});
