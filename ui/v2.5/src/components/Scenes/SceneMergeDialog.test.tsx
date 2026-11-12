import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import type { ReactNode } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import messages from "src/locales/en-GB.json";
import * as GQL from "src/core/generated-graphql";
import { SceneMergeModal } from "./SceneMergeDialog";

const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  effects: [] as (() => void)[],
  accept: undefined as undefined | { onClick: () => Promise<void> | void },
  buttons: [] as { onClick?: () => void; title?: string }[],
  query: vi.fn(),
  merge: vi.fn(),
  deleteFiles: vi.fn(),
  close: vi.fn(),
  checks: [] as {
    id?: string;
    checked?: boolean;
    onChange?: (event: { target: { checked: boolean } }) => void;
  }[],
  cancel: undefined as undefined | { onClick: () => void },
  error: vi.fn(),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useState: (initial: unknown) => {
    const i = page.next++;
    if (!(i in page.state))
      page.state[i] = typeof initial === "function" ? initial() : initial;
    return [
      page.state[i],
      (value: unknown) => {
        page.state[i] = value;
      },
    ];
  },
  useEffect: (effect: () => void) => {
    page.effects.push(effect);
  },
}));
vi.mock("src/core/StashService", () => ({
  queryFindFullScenesByID: page.query,
  mutateSceneMerge: page.merge,
  mutateDeleteFiles: page.deleteFiles,
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: page.error, success: vi.fn() }),
}));
vi.mock("src/hooks/CurrentUser", () => ({ useIsAdmin: () => true }));
vi.mock("src/hooks/Config", async () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
  ConfigurationContext: (await import("react")).createContext(undefined),
}));
vi.mock("src/utils/image", () => ({
  default: { imageToDataURL: async (url: string) => url },
}));
vi.mock("src/components/Scenes/SceneSelect", () => ({
  SceneSelect: () => null,
}));
vi.mock("src/components/Shared/Select", () => ({ GallerySelect: () => null }));
vi.mock("src/components/Shared/ScrapeDialog/ScrapedObjectsRow", () => ({
  ScrapedGroupsRow: () => null,
  ScrapedPerformersRow: () => null,
  ScrapedStudioRow: () => null,
  ScrapedTagsRow: () => null,
}));
vi.mock("src/components/Shared/Rating/RatingSystem", () => ({
  RatingSystem: () => null,
}));
vi.mock("src/components/Shared/StashID", () => ({ StashIDsField: () => null }));
vi.mock("src/components/Shared/Modal", () => ({
  ModalComponent: (props: {
    header: string;
    children: ReactNode;
    accept: typeof page.accept;
    cancel: typeof page.cancel;
  }) => {
    page.accept = props.accept;
    page.cancel = props.cancel;
    return (
      <>
        <h1>{props.header}</h1>
        {props.children}
      </>
    );
  },
}));
vi.mock("react-bootstrap", async (original) => ({
  ...(await original<typeof import("react-bootstrap")>()),
  Form: Object.assign(
    (await original<typeof import("react-bootstrap")>()).Form,
    {
      Check: (props: (typeof page.checks)[number] & { label?: string }) => {
        page.checks.push(props);
        return (
          <label>
            <input
              type="checkbox"
              id={props.id}
              checked={props.checked}
              readOnly
            />
            {props.label}
          </label>
        );
      },
    }
  ),
  Button: (props: {
    onClick?: () => void;
    title?: string;
    children?: ReactNode;
  }) => {
    page.buttons.push(props);
    return (
      <button type="button" title={props.title}>
        {props.children}
      </button>
    );
  },
}));
function flatten(object: object, prefix = ""): Record<string, string> {
  return Object.fromEntries(
    Object.entries(object).flatMap(([key, value]) => {
      const id = prefix ? `${prefix}.${key}` : key;
      return typeof value === "string"
        ? [[id, value]]
        : Object.entries(flatten(value, id));
    })
  );
}
const scenes = [
  {
    id: "1",
    title: "Puppies Playing",
    files: [
      {
        id: "f1",
        path: "C:\\videos\\puppies-1080.mp4",
        width: 1920,
        height: 1080,
      },
    ],
  },
  {
    id: "2",
    title: "Puppies Playing",
    files: [
      { id: "f2", path: "/videos/puppies-2160.mp4", width: 3840, height: 2160 },
    ],
  },
].map((scene) => ({
  ...scene,
  code: scene.id === "1" ? "destination code" : "source code",
  urls: [],
  paths: { screenshot: scene.id },
  performers: [],
  groups: [],
  tags: [],
  galleries: [],
  stash_ids: [],
  custom_fields: {},
})) as unknown as GQL.SceneDataFragment[];
function render(selected = scenes) {
  page.next = 0;
  page.buttons = [];
  page.checks = [];
  page.effects = [];
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={flatten(messages)}>
      <SceneMergeModal
        show
        scenes={selected.map((scene) => ({
          id: scene.id,
          title: scene.title ?? "",
        }))}
        onClose={page.close}
      />
    </IntlProvider>
  );
}
async function effects() {
  page.effects.forEach((effect) => {
    effect();
  });
  await Promise.resolve();
  await Promise.resolve();
}
beforeEach(() => {
  page.state = [];
  vi.resetAllMocks();
  page.merge.mockResolvedValue({ data: { sceneMerge: { id: "1" } } });
  page.deleteFiles.mockResolvedValue({ data: { deleteFiles: true } });
  vi.stubGlobal("document", { body: {} });
  page.query.mockResolvedValue({ data: { findScenes: { scenes } } });
});
it("names metadata merging and explains source and destination in order", () => {
  const html = render();
  expect(html).toContain("Merge metadata");
  expect(html).toContain("Scene to merge");
  expect(html).toContain("Target scene where source will be merged into");
  expect(html.indexOf('for="source"')).toBeLessThan(
    html.indexOf('for="destination"')
  );
});
it("identifies the destination by filename and resolution and updates it when swapped", async () => {
  render();
  await effects();
  let html = render();
  expect(html.slice(html.indexOf('for="destination"'))).toContain(
    "puppies-1080.mp4 (1080p)"
  );
  expect(html.slice(0, html.indexOf('for="destination"'))).toContain(
    "puppies-2160.mp4 (4K)"
  );
  page.buttons.find((b) => b.title === "Swap")!.onClick!();
  html = render();
  expect(html.slice(html.indexOf('for="destination"'))).toContain(
    "puppies-2160.mp4 (4K)"
  );
});
it.each([
  "",
  "puppies-2160.mp4",
])("shows the filename once without a bare id when the source title is %j", async (title) => {
  page.query.mockResolvedValue({
    data: {
      findScenes: { scenes: [scenes[0], { ...scenes[1], title }] },
    },
  });
  render();
  await effects();
  let html = render();
  const expectSourceIdentity = (markup: string) => {
    expect(markup).toContain("puppies-2160.mp4 (4K)");
    expect(markup.match(/puppies-2160\.mp4 \(4K\)/g)).toHaveLength(1);
    expect(markup).not.toContain(">puppies-2160.mp4</div>");
    expect(markup).not.toContain(">2<");
  };
  expectSourceIdentity(html.slice(0, html.indexOf('for="destination"')));

  await page.accept!.onClick();
  render();
  await effects();
  html = render();
  expectSourceIdentity(html);
});
it("keeps source before destination in step two and applies to the selected destination", async () => {
  render();
  await page.accept!.onClick();
  render();
  await effects();
  const html = render();
  expect(html).toContain("Merge metadata");
  expect(html.indexOf("puppies-2160.mp4")).toBeLessThan(
    html.indexOf("puppies-1080.mp4")
  );
  expect(html).toContain(">Combined<");
  expect(html.indexOf(">Combined<")).toBeLessThan(
    html.lastIndexOf(">Destination<")
  );
  expect(html.indexOf('value="source code"')).toBeLessThan(
    html.indexOf('value="destination code"')
  );
  page.buttons[0].onClick!();
  render();
  await page.accept!.onClick();
  expect(page.merge).toHaveBeenCalledWith(
    "1",
    ["2"],
    expect.objectContaining({ id: "1", code: "source code" }),
    expect.any(Boolean),
    expect.any(Boolean)
  );
});

async function prepareMerge(deleteFiles = false, selected = scenes) {
  render(selected);
  if (deleteFiles) {
    const check = page.checks.find((c) => c.id === "delete-source-files");
    expect(check, "delete source files option").toBeDefined();
    expect(check!.checked).toBe(false);
    check!.onChange!({ target: { checked: true } });
    render(selected);
  }
  await page.accept!.onClick();
  render(selected);
  await effects();
  render(selected);
  await page.accept!.onClick();
}

it("keeps every file by default", async () => {
  render();
  expect(page.checks.find((c) => c.id === "delete-source-files")?.checked).toBe(
    false
  );
  await prepareMerge();
  expect(page.merge).toHaveBeenCalledOnce();
  expect(page.deleteFiles).not.toHaveBeenCalled();
});

it("warns with source file paths before doing anything and can cancel", async () => {
  await prepareMerge(true);
  const html = render();
  expect(html).toContain("delete-dialog alert alert-danger");
  expect(html).toContain("<li>/videos/puppies-2160.mp4</li>");
  expect(html).not.toContain("puppies-1080.mp4");
  expect(page.merge).not.toHaveBeenCalled();
  expect(page.deleteFiles).not.toHaveBeenCalled();
  page.cancel!.onClick();
  render();
  expect(page.merge).not.toHaveBeenCalled();
  expect(page.deleteFiles).not.toHaveBeenCalled();
});

it("deletes all source files only after the merge succeeds", async () => {
  const selected = [
    ...scenes,
    {
      ...scenes[1],
      id: "3",
      files: [
        { ...scenes[1].files[0], id: "f3", path: "/videos/third.mp4" },
        { ...scenes[1].files[0], id: "f4", path: "/videos/fourth.mp4" },
      ],
    },
  ];
  page.query.mockResolvedValue({ data: { findScenes: { scenes: selected } } });
  let finishMerge!: (result: unknown) => void;
  page.merge.mockReturnValue(
    new Promise((resolve) => {
      finishMerge = resolve;
    })
  );
  await prepareMerge(true, selected);
  render(selected);
  const completion = page.accept!.onClick();
  expect(page.merge).toHaveBeenCalledOnce();
  expect(page.deleteFiles).not.toHaveBeenCalled();
  finishMerge({ data: { sceneMerge: { id: "1" } } });
  await completion;
  expect(page.deleteFiles).toHaveBeenCalledExactlyOnceWith(["f2", "f3", "f4"]);
  expect(page.close).toHaveBeenCalledWith("1");
});

it("excludes destination files shared with a source from the warning and deletion", async () => {
  const selected = [
    scenes[0],
    { ...scenes[1], files: [...scenes[0].files, ...scenes[1].files] },
  ];
  page.query.mockResolvedValue({ data: { findScenes: { scenes: selected } } });
  await prepareMerge(true, selected);
  const html = render(selected);
  expect(html).toContain("<li>/videos/puppies-2160.mp4</li>");
  expect(html).not.toContain("puppies-1080.mp4");
  await page.accept!.onClick();
  expect(page.deleteFiles).toHaveBeenCalledExactlyOnceWith(["f2"]);
});

it("merges without a warning when all source files belong to the destination", async () => {
  const selected = [scenes[0], { ...scenes[1], files: scenes[0].files }];
  page.query.mockResolvedValue({ data: { findScenes: { scenes: selected } } });
  await prepareMerge(true, selected);
  expect(render(selected)).not.toContain("delete-dialog alert alert-danger");
  expect(page.merge).toHaveBeenCalledOnce();
  expect(page.deleteFiles).not.toHaveBeenCalled();
  expect(page.close).toHaveBeenCalledExactlyOnceWith("1");
});

it("lists and deletes a file shared by multiple sources only once", async () => {
  const selected = [...scenes, { ...scenes[1], id: "3" }];
  page.query.mockResolvedValue({ data: { findScenes: { scenes: selected } } });
  await prepareMerge(true, selected);
  const html = render(selected);
  expect(html.match(/<li>\/videos\/puppies-2160\.mp4<\/li>/g)).toHaveLength(1);
  await page.accept!.onClick();
  expect(page.deleteFiles).toHaveBeenCalledExactlyOnceWith(["f2"]);
});

it("uses the sources after swapping the destination", async () => {
  render();
  page.buttons.find((b) => b.title === "Swap")!.onClick!();
  await prepareMerge(true);
  const html = render();
  expect(html).toContain("C:\\videos\\puppies-1080.mp4");
  expect(html).not.toContain("puppies-2160.mp4");
  await page.accept!.onClick();
  expect(page.deleteFiles).toHaveBeenCalledExactlyOnceWith(["f1"]);
  expect(page.close).toHaveBeenCalledWith("2");
});

it.each([
  "reject",
  "no result",
])("deletes nothing when merging fails: %s", async (failure) => {
  const error = new Error("Merge failed");
  if (failure === "reject") page.merge.mockRejectedValue(error);
  else page.merge.mockResolvedValue({ data: { sceneMerge: null } });
  await prepareMerge(true);
  render();
  await page.accept!.onClick();
  expect(page.deleteFiles).not.toHaveBeenCalled();
  expect(page.close).not.toHaveBeenCalled();
  if (failure === "reject") expect(page.error).toHaveBeenCalledWith(error);
});

it("shows a deletion error and closes with the completed merge", async () => {
  const error = new Error("Delete failed");
  page.deleteFiles.mockRejectedValue(error);
  await prepareMerge(true);
  render();
  await page.accept!.onClick();
  expect(page.merge).toHaveBeenCalledOnce();
  expect(page.error).toHaveBeenCalledWith(error);
  expect(page.close).toHaveBeenCalledExactlyOnceWith("1");
});
