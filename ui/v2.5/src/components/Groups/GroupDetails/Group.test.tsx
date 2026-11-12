import type { ComponentProps, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";
import type { FilteredSceneList } from "src/components/Scenes/SceneList";
import Group from "./Group";

const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  ui: {} as { showChildGroupContent?: boolean },
  list: undefined as ComponentProps<typeof FilteredSceneList> | undefined,
  toggle: undefined as { checked: boolean; onChange: () => void } | undefined,
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useState: (initial: unknown) => {
    const i = page.next++;
    if (!(i in page.state)) page.state[i] = initial;
    return [
      page.state[i],
      (value: unknown) => {
        page.state[i] = value;
      },
    ];
  },
}));
vi.mock("react-bootstrap", () => ({
  Tabs: ({ children }: { children: ReactNode }) => children,
  Tab: ({ children, eventKey }: { children: ReactNode; eventKey: string }) =>
    eventKey === "scenes" ? children : null,
  Form: {
    Check: (props: NonNullable<typeof page.toggle>) => {
      page.toggle = props;
      return null;
    },
  },
  Button: () => null,
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({}),
  Redirect: () => null,
}));
vi.mock("react-intl", () => ({
  FormattedMessage: () => null,
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("react-helmet", () => ({ Helmet: () => null }));
vi.mock("mousetrap", () => ({ default: {} }));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { ui: page.ui } }),
}));
vi.mock("src/core/StashService", () => ({
  useFindGroup: () => ({
    data: {
      findGroup: {
        id: "10",
        name: "Parent",
        scene_count: 0,
        sub_group_count: 1,
        sub_groups: [{ id: "11" }],
      },
    },
  }),
  useGroupUpdate: () => [vi.fn(), {}],
  useGroupUserDataUpdate: () => [vi.fn()],
  useGroupDestroy: () => [vi.fn(), {}],
  useBulkSceneUpdate: () => [vi.fn()],
  useScenesUpdate: () => [vi.fn()],
}));
vi.mock("src/components/Scenes/SceneList", () => ({
  FilteredSceneList: (props: ComponentProps<typeof FilteredSceneList>) => {
    page.list = props;
    return null;
  },
}));
vi.mock("./AddScenesDialog", () => ({ AddScenesDialog: () => null }));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({}) }));
vi.mock("src/hooks/modal", () => ({ useModal: () => ({}) }));
vi.mock("src/hooks/CurrentUser", () => ({ useCanModify: () => true }));
vi.mock("src/hooks/keybinds", () => ({ useRatingKeybinds: () => {} }));
vi.mock("src/hooks/detailsPanel", () => ({ useLoadStickyHeader: () => false }));
vi.mock("src/hooks/scrollToTop", () => ({ useScrollToTopOnMount: () => {} }));
vi.mock("src/components/List/keyboardShortcuts", async () => ({
  PageListShortcutsContext: (await import("react")).createContext({}),
  usePageListShortcuts: () => ({}),
}));
vi.mock("src/components/Shared/DetailsPage/Tabs", () => ({
  TabTitleCounter: () => null,
  useTabKey: () => ({ activeTabKey: "scenes", setTabKey: vi.fn() }),
}));
vi.mock("./GroupDetailsPanel", () => ({
  GroupDetailsPanel: () => null,
  CompressedGroupDetailsPanel: () => null,
}));
vi.mock("./GroupEditPanel", () => ({ GroupEditPanel: () => null }));
vi.mock("./GroupAudiosPanel", () => ({ GroupAudiosPanel: () => null }));
vi.mock("./GroupSubGroupsPanel", () => ({ GroupSubGroupsPanel: () => null }));
vi.mock("./GroupPerformersPanel", () => ({ GroupPerformersPanel: () => null }));
vi.mock("src/components/Shared/FavoriteIcon", () => ({
  FavoriteIcon: () => null,
}));
vi.mock("src/components/Shared/DetailsEditNavbar", () => ({
  DetailsEditNavbar: () => null,
}));
vi.mock("src/components/Shared/ErrorMessage", () => ({
  ErrorMessage: () => null,
}));
vi.mock("src/components/Shared/LoadingIndicator", () => ({
  LoadingIndicator: () => null,
}));
vi.mock("src/components/Shared/Modal", () => ({ ModalComponent: () => null }));
vi.mock("src/components/Shared/Rating/RatingSystem", () => ({
  RatingSystem: () => null,
}));
vi.mock("src/components/Shared/DetailImage", () => ({
  DetailImage: () => null,
}));
vi.mock("src/components/Shared/ExternalLinksButton", () => ({
  ExternalLinkButtons: () => null,
}));
vi.mock("src/components/Shared/DetailsPage/BackgroundImage", () => ({
  BackgroundImage: () => null,
}));
vi.mock("src/components/Shared/DetailsPage/DetailTitle", () => ({
  DetailTitle: () => null,
}));
vi.mock("src/components/Shared/CollapseButton", () => ({
  ExpandCollapseButton: () => null,
}));
vi.mock("src/components/Shared/DetailsPage/AliasList", () => ({
  AliasList: () => null,
}));
vi.mock("src/components/Shared/DetailsPage/HeaderImage", () => ({
  HeaderImage: () => null,
}));
vi.mock("src/hooks/Lightbox/LightboxLink", () => ({
  LightboxLink: () => null,
}));
vi.mock("src/components/Scenes/SceneDetails/OrganizedButton", () => ({
  OrganizedButton: () => null,
}));
vi.mock("src/components/Shared/Alert", () => ({ AlertModal: () => null }));
vi.mock("src/components/List/ItemList", () => ({
  showWhenSelected: () => null,
  showWhenNoneSelected: () => null,
}));

function render() {
  page.next = 0;
  page.toggle = undefined;
  renderToStaticMarkup(
    <Group
      {...({
        match: { params: { id: "10", tab: "scenes" } },
        location: {},
      } as ComponentProps<typeof Group>)}
    />
  );
}
function depth() {
  return page.list!.filterHook!(
    new ListFilterModel(GQL.FilterMode.Scenes)
  ).makeFilter().groups;
}
beforeEach(() => {
  page.state = [];
  page.ui = {};
});
it("switches the Scenes query between descendants and the direct group", () => {
  render();
  expect(page.toggle).toBeDefined();
  expect(page.toggle!.checked).toBe(false);
  expect(depth()).toMatchObject({ value: ["10"], depth: 0 });
  page.toggle!.onChange();
  render();
  expect(page.toggle!.checked).toBe(true);
  expect(depth()).toMatchObject({ value: ["10"], depth: -1 });
  expect(page.list!.onMove).toBeUndefined();
  page.toggle!.onChange();
  render();
  expect(depth()).toMatchObject({ value: ["10"], depth: 0 });
  expect(page.list!.onMove).toBeTypeOf("function");
});
it("starts from the saved interface default", () => {
  page.ui = { showChildGroupContent: true };
  render();
  expect(page.toggle).toBeDefined();
  expect(page.toggle!.checked).toBe(true);
  expect(depth()).toMatchObject({ value: ["10"], depth: -1 });
});
