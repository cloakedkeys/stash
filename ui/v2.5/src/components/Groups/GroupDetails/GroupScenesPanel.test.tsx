import type { ComponentProps, ReactElement, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import type { FilteredSceneList } from "src/components/Scenes/SceneList";
import type { IAlertModalProps } from "src/components/Shared/Alert";
import messages from "src/locales/en-GB.json";
import { GroupScenesPanel } from "./GroupScenesPanel";

const page = vi.hoisted(() => ({
  list: undefined as ComponentProps<typeof FilteredSceneList> | undefined,
  query: vi.fn(),
  reorder: vi.fn(),
  modal: undefined as ReactNode,
}));
vi.mock("src/components/Scenes/SceneList", () => ({
  FilteredSceneList: (props: ComponentProps<typeof FilteredSceneList>) => {
    page.list = props;
    return null;
  },
}));
vi.mock("src/core/StashService", () => ({
  queryFindScenes: page.query,
  useBulkSceneUpdate: () => [vi.fn()],
  useScenesUpdate: () => [page.reorder],
}));
vi.mock("src/components/List/ItemList", () => ({
  showWhenSelected: vi.fn(),
  showWhenNoneSelected: vi.fn(),
}));
vi.mock("./AddScenesDialog", () => ({ AddScenesDialog: () => null }));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({}) }));
vi.mock("src/hooks/modal", () => ({
  useModal: () => ({
    modal: null,
    showModal: (modal: ReactNode) => {
      page.modal = modal;
    },
    closeModal: () => {
      page.modal = undefined;
    },
  }),
}));

function scenes(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    id: String(i + 1),
    groups: [
      { group: { id: "10" }, scene_index: i + 1 },
      { group: { id: "20" }, scene_index: i % 2 ? null : 99 },
    ],
  }));
}
beforeEach(() => {
  vi.clearAllMocks();
  page.modal = undefined;
  page.query.mockResolvedValue({
    data: { findScenes: { scenes: scenes(300) } },
  });
  page.reorder.mockResolvedValue({});
  renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      onError={() => {}}
      messages={{
        "dialogs.renumber_group_scenes": messages.dialogs.renumber_group_scenes,
        "actions.continue": messages.actions.continue,
      }}
    >
      <GroupScenesPanel
        active
        showSubGroupContent
        group={{ id: "10", name: "Group" } as GQL.GroupDataFragment}
      />
    </IntlProvider>
  );
});
it("moves scene 1 to the end of a 300-scene group in one mutation", async () => {
  expect(page.list!.manualSortBy).toBe("group_scene_number");
  await page.list!.onMove!(["1"], "300", true, GQL.SortDirectionEnum.Asc);
  expect(page.reorder).toHaveBeenCalledOnce();
  const input = page.reorder.mock.calls[0][0].variables.input;
  expect(input.map((s: GQL.SceneUpdateInput) => s.id)).toEqual([
    ...Array.from({ length: 299 }, (_, i) => String(i + 2)),
    "1",
  ]);
  input.forEach((s: GQL.SceneUpdateInput, i: number) => {
    expect(s.groups).toEqual([
      { group_id: "10", scene_index: i + 1 },
      { group_id: "20", scene_index: Number(s.id) % 2 ? 99 : null },
    ]);
  });
  const filter = page.query.mock.calls[0][0];
  expect(filter.makeFindFilter()).toMatchObject({
    per_page: -1,
    page: 1,
    sort: "group_scene_number",
    direction: GQL.SortDirectionEnum.Asc,
  });
  expect(filter.makeFilter()).toEqual({
    groups: {
      value: ["10"],
      modifier: GQL.CriterionModifier.Includes,
      depth: 0,
      excludes: [],
    },
  });
});
it("moves multiple scenes before a target in descending order", async () => {
  page.query.mockResolvedValue({
    data: { findScenes: { scenes: scenes(4).reverse() } },
  });
  await page.list!.onMove!(["2", "1"], "4", false, GQL.SortDirectionEnum.Desc);
  expect(page.reorder).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: ["2", "1", "4", "3"].map((id, i) => ({
        id,
        groups: [
          { group_id: "10", scene_index: 4 - i },
          { group_id: "20", scene_index: Number(id) % 2 ? 99 : null },
        ],
      })),
    },
  });
});
it("does not change a descendant or a scene removed from the group", async () => {
  await expect(
    page.list!.onMove!(["999"], "1", false, GQL.SortDirectionEnum.Asc)
  ).rejects.toThrow();
  expect(page.reorder).not.toHaveBeenCalled();
});
it("does not renumber when dropping a selection onto itself", async () => {
  await page.list!.onMove!(["1", "2"], "2", true, GQL.SortDirectionEnum.Asc);
  expect(page.reorder).not.toHaveBeenCalled();
});

it.each([
  { ids: [] },
  { ids: [""] },
  { ids: ["", ""] },
])("quietly ignores an empty source list $ids", async ({ ids }) => {
  await expect(
    page.list!.onMove!(ids, "1", false, GQL.SortDirectionEnum.Asc)
  ).resolves.toBeUndefined();
  expect(page.query).not.toHaveBeenCalled();
  expect(page.reorder).not.toHaveBeenCalled();
  expect(page.modal).toBeUndefined();
});

it.each([
  { numbers: [1, 5, 10, null, 5], direction: GQL.SortDirectionEnum.Asc },
  { numbers: [1, 2, 4], direction: GQL.SortDirectionEnum.Asc },
  { numbers: [1, 2, 2], direction: GQL.SortDirectionEnum.Asc },
  { numbers: [1, 2, null], direction: GQL.SortDirectionEnum.Asc },
  { numbers: [5, 3, 1], direction: GQL.SortDirectionEnum.Desc },
])("confirms renumbering $numbers ($direction), supports Cancel and Continue, and skips confirmation for clean numbers", async ({
  numbers,
  direction,
}) => {
  const messy = scenes(numbers.length);
  messy.forEach((scene, i) => {
    scene.groups[0].scene_index = numbers[i];
  });
  page.query.mockResolvedValue({ data: { findScenes: { scenes: messy } } });
  let move = page.list!.onMove!(["3"], "1", false, direction);
  await vi.waitFor(() => expect(page.modal).toBeDefined());
  let dialog = (page.modal as ReactElement<IAlertModalProps>).props;
  expect(dialog.text).toBe(
    `Moving this scene renumbers all ${numbers.length} scenes in this group from 1 to ${numbers.length}. Scenes without a number get one, and gaps and repeated numbers are removed.`
  );
  expect(dialog.confirmButtonText).toBe("Continue");
  expect(page.reorder).not.toHaveBeenCalled();
  dialog.onCancel();
  await move;
  expect(page.reorder).not.toHaveBeenCalled();
  expect(page.modal).toBeUndefined();

  move = page.list!.onMove!(["3"], "1", false, direction);
  await vi.waitFor(() => expect(page.modal).toBeDefined());
  dialog = (page.modal as ReactElement<IAlertModalProps>).props;
  dialog.onConfirm();
  await move;
  expect(page.modal).toBeUndefined();
  expect(page.reorder).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: ["3", "1", "2", ...messy.slice(3).map((scene) => scene.id)].map(
        (id, i) => ({
          id,
          groups: [
            {
              group_id: "10",
              scene_index:
                direction === GQL.SortDirectionEnum.Desc
                  ? numbers.length - i
                  : i + 1,
            },
            { group_id: "20", scene_index: Number(id) % 2 ? 99 : null },
          ],
        })
      ),
    },
  });

  page.reorder.mockClear();
  const clean = scenes(numbers.length);
  if (direction === GQL.SortDirectionEnum.Desc) clean.reverse();
  page.query.mockResolvedValue({ data: { findScenes: { scenes: clean } } });
  await page.list!.onMove!(["3"], "1", false, direction);
  expect(page.reorder).toHaveBeenCalledOnce();
  expect(page.modal).toBeUndefined();
});
