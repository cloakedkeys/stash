import { expect, it } from "vitest";
import { faBookOpen } from "@fortawesome/free-solid-svg-icons";
import type {
  CriterionModifier,
  FilterMode,
  SortDirectionEnum,
  SavedFilterDataFragment,
} from "../../core/generated-graphql";
import {
  buildMainNavbarMenuItems,
  buildSavedFilterMenuItems,
  isMenuItemActive,
  splitHref,
  type INavbarMenuItem,
} from "./menuItems";
import type { ISavedFilterMenuItem } from "../../core/config";

const mangaFilter: SavedFilterDataFragment = {
  __typename: "SavedFilter",
  id: "7",
  mode: "GALLERIES" as FilterMode,
  name: "Manga",
  find_filter: {
    __typename: "SavedFindFilterType",
    q: "",
    page: 1,
    per_page: 40,
    sort: "date",
    direction: "ASC" as SortDirectionEnum,
  },
  object_filter: {
    tags: {
      value: ["12"],
      modifier: "INCLUDES_ALL" as CriterionModifier,
    },
  },
  ui_options: {
    display_mode: 2,
    zoom_index: 0,
  },
};

const configuredItems: ISavedFilterMenuItem[] = [
  {
    id: "manga-section",
    label: "Manga",
    filterId: "7",
    mode: "GALLERIES" as FilterMode,
  },
];

it("builds saved filter menus and highlights the matching item", () => {
  const [item] = buildSavedFilterMenuItems(configuredItems, [mangaFilter]);

  expect(item.name).toBe("saved-filter:manga-section");
  expect(item.label).toBe("Manga");
  expect(item.message).toBe(undefined);
  expect(item.icon).toBe(faBookOpen);
  expect(item.hotkey).toBe(undefined);
  expect(item.href).toMatch(/^\/galleries\?/);
  expect(item.href).toMatch(/sortby=date/);
  expect(item.href).toMatch(/sortdir=asc/);
  expect(item.href).toMatch(/z=0/);
  expect(item.href).toMatch(/c=/);
  expect(buildSavedFilterMenuItems(configuredItems, []).length).toBe(0);

  const builtInItems: INavbarMenuItem[] = [
    {
      name: "galleries",
      label: "Galleries",
      href: "/galleries",
      icon: faBookOpen,
    },
  ];

  const combinedItems = buildMainNavbarMenuItems(
    builtInItems,
    undefined,
    configuredItems,
    [mangaFilter]
  );

  expect(combinedItems.length).toBe(2);
  expect(combinedItems[0].name).toBe("galleries");
  expect(combinedItems[1].name).toBe("saved-filter:manga-section");

  // only one item is highlighted on a saved filter's page
  const [galleriesItem, mangaItem] = combinedItems;
  const mangaSearch = "?" + mangaItem.href.split("?")[1];
  const activeNames = (pathname: string, search: string) =>
    combinedItems
      .filter((i) => isMenuItemActive(i, combinedItems, pathname, search))
      .map((i) => i.name)
      .join(",");

  expect(activeNames("/galleries", mangaSearch)).toBe(mangaItem.name);
  expect(activeNames("/galleries", "")).toBe(galleriesItem.name);
  expect(activeNames("/galleries", "?sortby=title")).toBe(galleriesItem.name);
  expect(activeNames("/scenes", mangaSearch)).toBe("");

  // parameter order does not matter
  const reordered =
    "?" +
    new URLSearchParams(mangaSearch).toString().split("&").reverse().join("&");
  expect(activeNames("/galleries", reordered)).toBe(mangaItem.name);

  expect(splitHref("/galleries?a=1").pathname).toBe("/galleries");
  expect(splitHref("/galleries?a=1").search).toBe("a=1");
  expect(splitHref("/galleries").search).toBe("");
});
