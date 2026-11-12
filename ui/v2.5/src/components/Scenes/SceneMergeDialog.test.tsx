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
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: page.error, success: vi.fn() }),
}));
vi.mock("src/hooks/CurrentUser", () => ({ useIsAdmin: () => true }));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
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
  }) => {
    page.accept = props.accept;
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
function render() {
  page.next = 0;
  page.buttons = [];
  page.effects = [];
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={flatten(messages)}>
      <SceneMergeModal
        show
        scenes={scenes.map((scene) => ({
          id: scene.id,
          title: scene.title ?? "",
        }))}
        onClose={() => {}}
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
  vi.clearAllMocks();
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
