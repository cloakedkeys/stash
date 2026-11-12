import { renderToStaticMarkup } from "react-dom/server";
import type { ComponentProps } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import GroupCreate from "./GroupCreate";
import type { GroupEditPanel } from "./GroupEditPanel";

const page = vi.hoisted(() => ({
  search: "?from_scene_id=4",
  scene: undefined as undefined | Record<string, unknown>,
  panel: undefined as undefined | ComponentProps<typeof GroupEditPanel>,
  state: [] as unknown[],
  next: 0,
  effects: [] as (() => void)[],
  effectDeps: [] as unknown[],
  effectNext: 0,
  create: vi.fn(),
  update: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  image: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useState: (initial: unknown) => {
    const i = page.next++;
    if (!(i in page.state)) page.state[i] = initial;
    return [
      page.state[i],
      (v: unknown) => {
        page.state[i] = v;
      },
    ];
  },
  useRef: () => {
    const i = page.next++;
    if (!(i in page.state)) page.state[i] = { current: undefined };
    return page.state[i];
  },
  useEffect: (effect: () => void, deps: unknown[]) => {
    const i = page.effectNext++;
    const old = page.effectDeps[i] as unknown[] | undefined;
    if (!old || deps.some((d, j) => d !== old[j])) page.effects.push(effect);
    page.effectDeps[i] = deps;
  },
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({ push: page.push, replace: page.replace }),
  useLocation: () => ({ search: page.search }),
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("src/core/StashService", () => ({
  useFindScene: (id: string) => ({
    loading: false,
    data: id === "new" ? undefined : { findScene: page.scene },
  }),
  useGroupCreate: () => [page.create],
  useSceneUpdate: () => [page.update],
}));
vi.mock("./GroupEditPanel", () => ({
  GroupEditPanel: (props: ComponentProps<typeof GroupEditPanel>) => {
    page.panel = props;
    return null;
  },
}));
vi.mock("src/components/Shared/LoadingIndicator", () => ({
  LoadingIndicator: () => null,
}));
vi.mock("src/utils/image", () => ({ default: { imageToDataURL: page.image } }));
vi.mock("src/hooks/Toast", () => {
  const toast = { success: page.success, error: page.error };
  return { useToast: () => toast };
});
function render() {
  page.next = 0;
  page.effectNext = 0;
  page.panel = undefined;
  renderToStaticMarkup(<GroupCreate />);
  page.effects.splice(0).forEach((effect) => {
    effect();
  });
}
async function load() {
  render();
  await new Promise((resolve) => setTimeout(resolve, 0));
  render();
}
beforeEach(() => {
  vi.clearAllMocks();
  page.state = [];
  page.effects = [];
  page.effectDeps = [];
  page.search = "?from_scene_id=4";
  page.scene = {
    id: "4",
    title: "Full movie",
    date: "2020-08-31",
    studio: { id: "2", name: "Studio" },
    director: "Director",
    urls: ["https://example.com/movie", "https://example.org/movie"],
    paths: { screenshot: "/scene/4/screenshot" },
    groups: [
      { group: { id: "8" }, scene_index: 3 },
      { group: { id: "9" }, scene_index: null },
    ],
  };
  page.image.mockResolvedValue("data:image/jpeg;base64,Y292ZXI=");
  page.create.mockResolvedValue({ data: { groupCreate: { id: "12" } } });
  page.update.mockResolvedValue({ data: { sceneUpdate: { id: "4" } } });
});
it("pre-fills the existing group form without saving anything", async () => {
  await load();
  expect(page.panel?.group).toEqual({
    name: "Full movie",
    date: "2020-08-31",
    studio: { id: "2", name: "Studio" },
    director: "Director",
    urls: ["https://example.com/movie", "https://example.org/movie"],
    front_image: "data:image/jpeg;base64,Y292ZXI=",
  });
  expect(page.image).toHaveBeenCalledWith("/scene/4/screenshot");
  expect(page.create).not.toHaveBeenCalled();
  expect(page.update).not.toHaveBeenCalled();
});
it.each([
  null,
  "",
])("pre-fills an untitled scene's file-name title when title is %j", async (title) => {
  page.scene = {
    ...page.scene,
    title,
    files: [{ path: "/library/movies/Full movie.mp4" }],
  };
  await load();
  expect(page.panel?.group?.name).toBe("Full movie.mp4");
});
it("keeps absent scene fields empty", async () => {
  page.scene = {
    id: "4",
    title: null,
    date: null,
    studio: null,
    director: null,
    urls: [],
    paths: {},
    groups: [],
  };
  await load();
  expect(page.panel?.group).toEqual({
    name: "",
    date: null,
    studio: null,
    director: null,
    urls: [],
    front_image: undefined,
  });
  expect(page.image).not.toHaveBeenCalled();
});
it("saves edited values before linking scene 1, preserving memberships and indices", async () => {
  await load();
  page.create.mockRejectedValueOnce(new Error("create failed"));
  await expect(page.panel!.onSubmit({ name: "Movie" })).rejects.toThrow(
    "create failed"
  );
  expect(page.update).not.toHaveBeenCalled();
  page.create.mockImplementationOnce(async () => {
    expect(page.update).not.toHaveBeenCalled();
    return { data: { groupCreate: { id: "12" } } };
  });
  await page.panel!.onSubmit({
    name: "Edited name",
    front_image: "edited cover",
  });
  expect(page.create).toHaveBeenCalledWith({
    variables: { input: { name: "Edited name", front_image: "edited cover" } },
  });
  expect(page.update).toHaveBeenCalledWith({
    variables: {
      input: {
        id: "4",
        groups: [
          { group_id: "8", scene_index: 3 },
          { group_id: "9", scene_index: null },
          { group_id: "12", scene_index: 1 },
        ],
      },
    },
  });
  expect(page.push).toHaveBeenCalledWith("/groups/12");
});
it("retries a failed link without creating a second group", async () => {
  await load();
  page.update.mockRejectedValueOnce(new Error("link failed"));
  await expect(page.panel!.onSubmit({ name: "Movie" })).rejects.toThrow(
    "link failed"
  );
  expect(page.push).not.toHaveBeenCalled();
  expect(page.success).not.toHaveBeenCalled();
  render();
  await page.panel!.onSubmit({ name: "Movie" });
  expect(page.create).toHaveBeenCalledTimes(1);
  expect(page.update).toHaveBeenCalledTimes(2);
});
it("save and new detaches the source scene after linking", async () => {
  await load();
  await page.panel!.onSubmit({ name: "Movie" }, true);
  expect(page.update).toHaveBeenCalledTimes(1);
  expect(page.replace).toHaveBeenCalledWith("/groups/new");
  expect(page.push).not.toHaveBeenCalled();
});
it("returns to ordinary creation after save and new", async () => {
  await load();
  await page.panel!.onSubmit({ name: "Movie" }, true);
  expect(page.update).toHaveBeenCalledTimes(1);
  expect(page.replace).toHaveBeenCalledWith("/groups/new");
  page.search = "?q=Normal";
  await load();
  expect(page.panel?.group).toEqual({ name: "Normal" });
  await page.panel!.onSubmit({ name: "Normal" });
  expect(page.update).toHaveBeenCalledTimes(1);
});
