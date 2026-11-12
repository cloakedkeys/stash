import { type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, expect, it, vi } from "vitest";

import type { Gallery } from "../Galleries/GallerySelect";
import messages from "src/locales/en-GB.json";

const state = vi.hoisted(() => ({
  ui: {} as { recentImageGalleries?: string[] },
  update: vi.fn(),
  save: vi.fn(),
  query: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
  close: vi.fn(),
  select: undefined as undefined | ((galleries: Gallery[]) => void),
  buttons: [] as {
    children: ReactNode;
    onClick: () => Promise<void>;
    disabled: boolean;
  }[],
  values: [] as unknown[],
  cursor: 0,
  effects: [] as (() => unknown)[],
}));
vi.mock("react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react")>();
  return {
    ...actual,
    useState: (initial: unknown) => {
      const index = state.cursor++;
      if (!(index in state.values)) state.values[index] = initial;
      return [
        state.values[index],
        (value: unknown) => {
          state.values[index] = value;
        },
      ];
    },
    useEffect: (effect: () => unknown) => {
      state.effects.push(effect);
    },
  };
});
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { ui: state.ui } }),
}));
vi.mock("src/core/StashService", () => ({
  useBulkImageUpdate: () => [state.update],
  useConfigureUISetting: () => [state.save],
  queryFindGalleriesByIDForSelect: (...args: unknown[]) => state.query(...args),
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: state.error, success: state.success }),
}));
vi.mock("../Shared/Modal", () => ({
  ModalComponent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}));
vi.mock("react-bootstrap", () => ({
  Button: (props: (typeof state.buttons)[number]) => {
    state.buttons.push(props);
    return <button disabled={props.disabled}>{props.children}</button>;
  },
}));
vi.mock("../Galleries/GallerySelect", () => ({
  GallerySelect: ({ onSelect }: { onSelect: typeof state.select }) => {
    state.select = onSelect;
    return <div>Gallery search</div>;
  },
}));
let AddToGalleryDialog: typeof import("./AddToGalleryDialog").AddToGalleryDialog;
const gallery = (id: string): Gallery => ({
  id,
  title: `Gallery ${id}`,
  files: [],
});
function render(ids = ["image-1", "image-2"]) {
  state.cursor = 0;
  state.buttons = [];
  state.effects = [];
  return renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      messages={{
        "actions.add_to_gallery": messages.actions.add_to_gallery,
        recently_used_galleries: messages.recently_used_galleries,
        "toast.images_added_to_gallery": messages.toast.images_added_to_gallery,
      }}
      onError={() => {}}
    >
      <MemoryRouter>
        <AddToGalleryDialog imageIds={ids} onClose={state.close} />
      </MemoryRouter>
    </IntlProvider>
  );
}
async function loadRecents(ids?: string[]) {
  render(ids);
  for (const effect of state.effects) effect();
  await vi.waitFor(() => expect(state.query).toHaveBeenCalled());
  await Promise.resolve();
  return render(ids);
}
beforeEach(async () => {
  AddToGalleryDialog = (await import("./AddToGalleryDialog"))
    .AddToGalleryDialog;
  vi.clearAllMocks();
  state.ui = {};
  state.values = [];
  state.update.mockResolvedValue({});
  state.save.mockImplementation(async ({ variables }) => {
    state.ui = { ...state.ui, [variables.key]: variables.value };
  });
  state.query.mockResolvedValue({ data: { findGalleries: { galleries: [] } } });
});

it.each([
  ["image-1", "image-2"],
  ["single-image"],
])("adds selected IDs %j using only ADD and shows the count and gallery link", async (...ids) => {
  render(ids);
  state.select!([gallery("g")]);
  await vi.waitFor(() => expect(state.close).toHaveBeenCalledWith(true));
  expect(state.update).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: {
        ids,
        gallery_ids: { ids: ["g"], mode: "ADD" },
      },
    },
  });
  const toast = renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={{}}>
      <MemoryRouter>{state.success.mock.calls[0][0]}</MemoryRouter>
    </IntlProvider>
  );
  expect(toast).toContain(
    `Added ${ids.length} image${ids.length === 1 ? "" : "s"} to gallery`
  );
  expect(toast).toContain('href="/galleries/g"');
  expect(state.ui.recentImageGalleries).toEqual(["g"]);
});

it("lists saved recent galleries in use order before search, ignoring deleted galleries", async () => {
  state.ui.recentImageGalleries = ["c", "missing", "a"];
  state.query.mockResolvedValue({
    data: { findGalleries: { galleries: [gallery("a"), gallery("c")] } },
  });
  const markup = await loadRecents();
  expect(state.query).toHaveBeenCalledWith(["c", "missing", "a"]);
  expect(state.buttons.map((b) => b.children)).toEqual([
    "Gallery c",
    "Gallery a",
  ]);
  expect(markup.indexOf("Gallery c")).toBeLessThan(markup.indexOf("Gallery a"));
  expect(markup.indexOf("Gallery a")).toBeLessThan(
    markup.indexOf("Gallery search")
  );
  await state.buttons[1].onClick();
  expect(state.update.mock.calls[0][0].variables.input.gallery_ids).toEqual({
    ids: ["a"],
    mode: "ADD",
  });
});

it("moves reused galleries to the front without duplicates and keeps only five across reopen", async () => {
  state.ui.recentImageGalleries = ["a", "b", "c", "d", "e"];
  render();
  state.select!([gallery("c")]);
  await vi.waitFor(() => expect(state.close).toHaveBeenCalled());
  expect(state.ui.recentImageGalleries).toEqual(["c", "a", "b", "d", "e"]);
  render();
  state.select!([gallery("f")]);
  await vi.waitFor(() => expect(state.close).toHaveBeenCalledTimes(2));
  expect(state.save).toHaveBeenLastCalledWith({
    variables: {
      key: "recentImageGalleries",
      value: ["f", "c", "a", "b", "d"],
    },
  });
  state.values = [];
  state.query.mockResolvedValue({
    data: {
      findGalleries: { galleries: ["a", "b", "c", "d", "f"].map(gallery) },
    },
  });
  await loadRecents();
  expect(state.buttons.map((b) => b.children)).toEqual([
    "Gallery f",
    "Gallery c",
    "Gallery a",
    "Gallery b",
    "Gallery d",
  ]);
});

it("keeps the picker and recents unchanged after an update error and allows retry", async () => {
  state.update.mockRejectedValueOnce(new Error("update failed"));
  render();
  state.select!([gallery("g")]);
  await vi.waitFor(() => expect(state.error).toHaveBeenCalled());
  expect(state.save).not.toHaveBeenCalled();
  expect(state.success).not.toHaveBeenCalled();
  expect(state.close).not.toHaveBeenCalled();
  state.select!([gallery("g")]);
  await vi.waitFor(() => expect(state.close).toHaveBeenCalledWith(true));
});

it("does not repeat an in-flight update or report a successful add as failed when recent saving fails", async () => {
  state.save.mockRejectedValueOnce(new Error("config failed"));
  render();
  state.select!([gallery("g")]);
  state.select!([gallery("g")]);
  await vi.waitFor(() => expect(state.close).toHaveBeenCalledWith(true));
  expect(state.update).toHaveBeenCalledTimes(1);
  expect(state.success).toHaveBeenCalledTimes(1);
  expect(state.error).toHaveBeenCalledTimes(1);
});
