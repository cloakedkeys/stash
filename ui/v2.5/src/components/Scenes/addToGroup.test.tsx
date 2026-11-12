import type { ComponentProps, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import messages from "src/locales/en-GB.json";
import { ListFilterModel } from "src/models/list-filter/filter";
import { GroupScenesPanel } from "../Groups/GroupDetails/GroupScenesPanel";
import { FilteredSceneList } from "./SceneList";

import type { Scene, SceneSelect } from "./SceneSelect";
import type { NumberField } from "src/utils/form";
import type { ModalComponent } from "src/components/Shared/Modal";

const page = vi.hoisted(() => ({
  update: vi.fn(),
  updateScene: vi.fn(),
  query: vi.fn(),
  state: [] as unknown[],
  cursor: 0,
  dialogRender: false,
  picker: undefined as ComponentProps<typeof SceneSelect> | undefined,
  field: undefined as ComponentProps<typeof NumberField> | undefined,
  dialog: undefined as ComponentProps<typeof ModalComponent> | undefined,
  refetch: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  canModify: true,
  selected: new Set(["scene-1", "scene-3", "scene-5"]),
  modal: undefined as ReactNode,
  buttons: [] as { title?: string; onClick: () => void }[],
}));
vi.mock("react", async (original) => {
  const actual = await original<typeof import("react")>();
  return {
    ...actual,
    useState: (initial: unknown) => {
      const state = actual.useState(initial);
      if (!page.dialogRender) return state;
      const index = page.cursor++;
      if (!(index in page.state)) page.state[index] = initial;
      return [
        page.state[index],
        (value: unknown) => {
          page.state[index] = value;
        },
      ];
    },
  };
});
vi.mock("src/components/Scenes/SceneSelect", () => ({
  SceneSelect: (props: ComponentProps<typeof SceneSelect>) => {
    page.picker = props;
    return <div>Scene search</div>;
  },
}));
vi.mock("src/utils/form", () => ({
  NumberField: (props: ComponentProps<typeof NumberField>) => {
    page.field = props;
    return (
      <input readOnly value={props.value} aria-invalid={props.isInvalid} />
    );
  },
}));
vi.mock("src/components/Shared/Modal", () => ({
  ModalComponent: (props: ComponentProps<typeof ModalComponent>) => {
    page.dialog = props;
    return <div>{props.children}</div>;
  },
}));
vi.mock("src/core/StashService", () => ({
  useBulkSceneUpdate: () => [page.update],
  useScenesUpdate: () => [vi.fn()],
  useSceneUpdate: () => [page.updateScene],
  queryFindScenesByID: (...args: unknown[]) => page.query(...args),
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

function addButton() {
  return page.buttons.find((b) => b?.title === messages.actions.add_scenes);
}
function renderDialog() {
  page.dialogRender = true;
  page.cursor = 0;
  try {
    return renderToStaticMarkup(
      <IntlProvider
        locale="en-GB"
        messages={{
          "actions.add_scenes": messages.actions.add_scenes,
          "group_add_scenes.starting_number":
            messages.group_add_scenes.starting_number,
          "group_add_scenes.invalid_number":
            messages.group_add_scenes.invalid_number,
          "group_add_scenes.missing_scene":
            messages.group_add_scenes.missing_scene,
          "toast.scenes_added_to_group": messages.toast.scenes_added_to_group,
        }}
        onError={() => {}}
      >
        {page.modal}
      </IntlProvider>
    );
  } finally {
    page.dialogRender = false;
  }
}
async function open() {
  render();
  expect(addButton()).toBeDefined();
  await addButton()!.onClick();
  expect(renderDialog()).toContain("Scene search");
  expect(page.picker?.isMulti).toBe(true);
}
function pick(ids = ["3", "1", "2"]) {
  page.picker!.onSelect!(ids.map((id) => ({ id, title: id }) as Scene));
  renderDialog();
}
function number(value: string, badInput = false) {
  page.field!.onChange!({
    currentTarget: { value, validity: { badInput } },
  } as Parameters<
    NonNullable<ComponentProps<typeof NumberField>["onChange"]>
  >[0]);
  renderDialog();
}
beforeEach(() => {
  vi.stubGlobal("document", { body: undefined });
  vi.clearAllMocks();
  page.state = [];
  page.dialogRender = false;
  page.modal = undefined;
  page.canModify = true;
  page.selected = new Set();
  page.update.mockResolvedValue({});
  page.updateScene.mockResolvedValue({});
  page.refetch.mockResolvedValue({});
  // The server returns a different order from the picker.
  page.query.mockResolvedValue({
    data: {
      findScenes: {
        scenes: [
          { id: "1", groups: [{ group: { id: "group-10" }, scene_index: 99 }] },
          { id: "2", groups: [{ group: { id: "20" }, scene_index: null }] },
          {
            id: "3",
            groups: [
              { group: { id: "30" }, scene_index: 7 },
              { group: { id: "40" }, scene_index: null },
            ],
          },
        ],
      },
    },
  });
});
it("adds without numbers using bulk ADD, skips existing members, reports counts and refreshes", async () => {
  await open();
  pick();
  await page.dialog!.accept!.onClick!();
  expect(page.query).toHaveBeenCalledExactlyOnceWith([3, 1, 2]);
  expect(page.update).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: {
        ids: ["3", "2"],
        group_ids: { ids: ["group-10"], mode: GQL.BulkUpdateIdMode.Add },
      },
    },
  });
  expect(page.updateScene).not.toHaveBeenCalled();
  expect(page.success).toHaveBeenCalledExactlyOnceWith(
    "2 added, 1 was already in the group"
  );
  expect(page.refetch).toHaveBeenCalledOnce();
  expect(page.modal).toBeUndefined();
});
it("numbers new members in pick order and preserves all other memberships and numbers", async () => {
  await open();
  pick();
  number("5");
  await page.dialog!.accept!.onClick!();
  expect(page.updateScene.mock.calls).toEqual([
    [
      {
        variables: {
          input: {
            id: "3",
            groups: [
              { group_id: "30", scene_index: 7 },
              { group_id: "40", scene_index: null },
              { group_id: "group-10", scene_index: 5 },
            ],
          },
        },
      },
    ],
    [
      {
        variables: {
          input: {
            id: "2",
            groups: [
              { group_id: "20", scene_index: null },
              { group_id: "group-10", scene_index: 6 },
            ],
          },
        },
      },
    ],
  ]);
  expect(page.update).not.toHaveBeenCalled();
  expect(page.success).toHaveBeenCalledExactlyOnceWith(
    "2 added, 1 was already in the group"
  );
  expect(page.refetch).toHaveBeenCalledOnce();
});
it.each([
  [["1"], "1 scene was already in the group; nothing added."],
  [["1", "4"], "2 scenes were already in the group; nothing added."],
])("reports an all-existing selection %j without modifying any scene", async (ids, message) => {
  await open();
  page.query.mockResolvedValueOnce({
    data: {
      findScenes: {
        scenes: (ids as string[]).map((id) => ({
          id,
          groups: [{ group: { id: "group-10" }, scene_index: 99 }],
        })),
      },
    },
  });
  pick(ids as string[]);
  number("5");
  await page.dialog!.accept!.onClick!();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.updateScene).not.toHaveBeenCalled();
  expect(page.success).toHaveBeenCalledExactlyOnceWith(message);
});
it.each([
  [["2"], "Added 1 scene to the group"],
  [["2", "3", "4"], "Added 3 scenes to the group"],
])("reports only the added count for new scenes %j", async (ids, message) => {
  await open();
  page.query.mockResolvedValueOnce({
    data: {
      findScenes: {
        scenes: (ids as string[]).map((id) => ({ id, groups: [] })),
      },
    },
  });
  pick(ids as string[]);
  await page.dialog!.accept!.onClick!();
  expect(page.success).toHaveBeenCalledExactlyOnceWith(message);
});
it("offers adding only on a group page with no selection and modify access", () => {
  render();
  expect(addButton()).toBeDefined();
  render(false);
  expect(addButton()).toBeUndefined();
  page.canModify = false;
  render();
  expect(addButton()).toBeUndefined();
  page.canModify = true;
  page.selected.add("1");
  render();
  expect(addButton()).toBeUndefined();
});
it("cancels without querying or updating", async () => {
  await open();
  pick();
  page.dialog!.cancel!.onClick!();
  expect(page.modal).toBeUndefined();
  expect(page.query).not.toHaveBeenCalled();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.refetch).not.toHaveBeenCalled();
});
it("reports update failure and keeps the dialog open for retry", async () => {
  await open();
  pick();
  const error = new Error("Update failed");
  page.update.mockRejectedValue(error);
  await page.dialog!.accept!.onClick!();
  expect(page.error).toHaveBeenCalledExactlyOnceWith(error);
  expect(page.success).not.toHaveBeenCalled();
  expect(page.modal).toBeDefined();
  renderDialog();
  expect(page.dialog!.isRunning).toBe(false);
});
it.each([
  ["1.5", false],
  ["", true],
  ["2147483648", false],
])("rejects invalid starting number %s (badInput %s)", async (value, badInput) => {
  await open();
  pick();
  number(value as string, badInput as boolean);
  expect(renderDialog()).toContain("Enter a whole number.");
  expect(page.dialog!.disabled).toBe(true);
  await page.dialog!.accept!.onClick!();
  expect(page.query).not.toHaveBeenCalled();
  expect(page.update).not.toHaveBeenCalled();
});
it("disables adding an empty selection", async () => {
  await open();
  expect(page.dialog!.disabled).toBe(true);
  await page.dialog!.accept!.onClick!();
  expect(page.query).not.toHaveBeenCalled();
});

it("rejects a sequence that would overflow before updating any scene", async () => {
  await open();
  pick();
  number("2147483647");
  await page.dialog!.accept!.onClick!();
  expect(page.updateScene).not.toHaveBeenCalled();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.error.mock.calls[0][0].message).toBe("Enter a whole number.");
  expect(page.success).not.toHaveBeenCalled();
  expect(page.modal).toBeDefined();
});

it.each(["0", "-1"])("accepts a starting number of %s", async (value) => {
  await open();
  pick(["2"]);
  number(value);
  expect(page.dialog!.disabled).toBe(false);
  await page.dialog!.accept!.onClick!();
  expect(page.updateScene).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: {
        id: "2",
        groups: [
          { group_id: "20", scene_index: null },
          { group_id: "group-10", scene_index: Number(value) },
        ],
      },
    },
  });
  expect(page.success).toHaveBeenCalledExactlyOnceWith(
    "Added 1 scene to the group"
  );
});

it("keeps the dialog open without updates if a selected scene cannot be loaded", async () => {
  await open();
  pick(["4"]);
  await page.dialog!.accept!.onClick!();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.updateScene).not.toHaveBeenCalled();
  expect(page.error.mock.calls[0][0].message).toBe(
    messages.group_add_scenes.missing_scene
  );
  expect(page.success).not.toHaveBeenCalled();
  expect(page.modal).toBeDefined();
});
