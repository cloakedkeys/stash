import React, { isValidElement, type ReactElement } from "react";
import { Dropdown } from "react-bootstrap";
import { GalleryEditPanel } from "../../Galleries/GalleryDetails/GalleryEditPanel";
import { GalleryScrapeDialog } from "../../Galleries/GalleryDetails/GalleryScrapeDialog";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactNode } from "react";
import * as GQL from "src/core/generated-graphql";
import { SceneEditPanel } from "./SceneEditPanel";
import { SceneScrapeDialog } from "./SceneScrapeDialog";

vi.mock("src/components/Scenes/SceneSelect", () => ({
  SceneSelect: () => null,
}));
vi.mock("src/hooks/state", async () => {
  const { useState } = await import("react");
  return {
    useInitialState: (value: unknown) => {
      const [state, setState] = useState(value);
      return [state, setState, setState];
    },
  };
});

const hooks = vi.hoisted(() => ({
  states: [] as unknown[],
  index: 0,
  effects: [] as (() => void)[],
  dependencies: [] as unknown[],
  effectIndex: 0,
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof React>()),
  useState: (initial: unknown) => {
    const index = hooks.index++;
    if (!(index in hooks.states)) hooks.states[index] = initial;
    return [
      hooks.states[index],
      (value: unknown) => {
        hooks.states[index] = value;
      },
    ];
  },
  useEffect: (effect: () => void, dependencies?: unknown[]) => {
    const index = hooks.effectIndex++;
    if (
      JSON.stringify(hooks.dependencies[index]) !== JSON.stringify(dependencies)
    ) {
      hooks.dependencies[index] = dependencies;
      hooks.effects.push(effect);
    }
  },
  useMemo: (fn: () => unknown) => fn(),
}));
const page = vi.hoisted(() => ({
  query: vi.fn(),
  image: vi.fn(),
  values: undefined as undefined | Record<string, unknown>,
  submit: undefined as undefined | (() => void),
  onSubmit: vi.fn(),
  error: vi.fn(),
}));

vi.mock("formik", () => ({
  useFormik: (options: {
    initialValues: Record<string, unknown>;
    onSubmit: (values: Record<string, unknown>) => void;
  }) => {
    page.values ??= { ...options.initialValues };
    page.submit = () => options.onSubmit(page.values!);
    return {
      values: page.values,
      initialValues: options.initialValues,
      dirty: true,
      errors: {},
      setFieldValue: (field: string, value: unknown) => {
        page.values![field] = value;
      },
      submitForm: page.submit,
      resetForm: vi.fn(),
    };
  },
}));
vi.mock("src/patch", () => ({
  PatchComponent: (_: string, component: unknown) => component,
  PatchFunction: (_: string, fn: unknown) => fn,
}));
vi.mock("src/components/Shared/ScrapeDialog/createObjects", () => ({
  useCreateScrapedStudio: () => vi.fn(),
  useCreateScrapedPerformer: () => vi.fn(),
  useCreateScrapedGroup: () => vi.fn(),
  useCreateScrapedTag: () => vi.fn(),
  useLinkScrapedTag: () => vi.fn(),
}));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanAdminister: () => false,
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: undefined }),
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: page.error }),
}));
vi.mock("react-router-dom", () => ({
  Prompt: () => null,
  useLocation: () => ({ pathname: "/" }),
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
  FormattedMessage: () => null,
}));
vi.mock("mousetrap", () => ({ default: {} }));
vi.mock("src/utils/lazyComponent", () => ({
  lazyComponent: () => "lazy-dialog",
}));
vi.mock("src/core/StashService", () => ({
  useListSceneScrapers: () => ({ data: undefined }),
  useListGalleryScrapers: () => ({ data: undefined }),
  getClient: () => ({ query: page.query }),
  useTagCreate: () => [vi.fn()],
  useTagUpdate: () => [vi.fn()],
}));
vi.mock("src/utils/image", () => ({
  default: { usePasteImage: () => undefined, imageToDataURL: page.image },
}));
vi.mock("src/utils/stashIds", () => ({
  getStashIDs: () => [],
}));
vi.mock("src/utils/yup", async () => {
  const yup = await import("yup");
  return {
    yupUniqueStringList: () => yup.array(yup.string()).defined(),
    yupDateString: () => yup.string(),
    yupFormikValidate: () => () => ({}),
  };
});
vi.mock("src/utils/form", () => ({
  formikUtils: () => ({
    renderField: (_field: string, _title: string, control: ReactNode) =>
      control,
    renderInputField: () => null,
    renderDateField: () => null,
    renderURLListField: () => null,
    renderStashIDsField: () => null,
  }),
}));
vi.mock("src/hooks/tagsEdit", () => ({
  useTagsEdit: () => ({
    tags: [],
    tagsControl: () => null,
    markTagsStateSaved: vi.fn(),
    resetTagsState: vi.fn(),
    updateTagsStateFromScraper: (tags: { stored_id: string }[]) => {
      page.values!.tag_ids = tags.map((t) => t.stored_id);
    },
  }),
}));
vi.mock("./usePerformerPictureFromFrame", () => ({
  usePerformerPictureFromFrame: () => ({}),
}));
vi.mock("src/components/Shared/CustomFields", () => ({
  CustomFieldsInput: () => null,
  formatCustomFieldInput: () => ({}),
}));
vi.mock("src/components/Shared/Icon", () => ({ Icon: () => null }));
vi.mock("src/components/Shared/ImageInput", () => ({
  ImageInput: () => null,
}));
vi.mock("src/components/Shared/ScraperMenu", () => ({
  ScraperMenu: () => null,
}));
vi.mock("src/components/Shared/StashBoxIDSearchModal", () => ({
  default: () => null,
}));
vi.mock("src/components/Performers/PerformerSelect", () => ({
  PerformerSelect: () => null,
}));
vi.mock("src/components/VoiceActors/VoiceActorSelect", () => ({
  VoiceActorSelect: () => null,
}));
vi.mock("src/components/Studios/StudioSelect", () => ({
  StudioSelect: () => null,
}));
vi.mock("src/components/Galleries/GallerySelect", () => ({
  GallerySelect: () => null,
}));
vi.mock("./SceneGroupTable", () => ({
  SceneGroupTable: () => null,
}));

// Inspect elements and invoke UI handlers, following the existing scrape dialog tests.
type Element = ReactElement<Record<string, unknown>>;
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements);
  if (!isValidElement<Record<string, unknown>>(node)) return [];
  return [node, ...elements(node.props.children as ReactNode)];
}
function render(fn: () => ReactNode): Element[] {
  hooks.index = 0;
  hooks.effectIndex = 0;
  const result = elements(fn());
  for (const effect of hooks.effects.splice(0)) effect();
  return result;
}

beforeEach(() => {
  hooks.states = [];
  hooks.effects = [];
  hooks.dependencies = [];
  page.values = undefined;
  vi.clearAllMocks();
  page.onSubmit.mockResolvedValue(undefined);
  page.image.mockResolvedValue("data:image/jpeg;base64,cover");
});

const source = {
  id: "23",
  title: "Linked title",
  code: "CODE",
  date: "2026-01-02",
  details: "Linked details",
  director: "Director",
  photographer: "Photographer",
  urls: ["https://example.com/linked"],
  studio: { id: "4", name: "Existing studio" },
  performers: [{ id: "5", name: "Existing performer" }],
  tags: [{ id: "6", name: "Existing tag" }],
  paths: { cover: "/gallery/23/cover" },
};

describe.each([
  "scene",
  "gallery",
] as const)("copy into %s edit form", (kind) => {
  const other = kind === "scene" ? "Gallery" : "Scene";
  const linkField = kind === "scene" ? "galleries" : "scenes";
  function panel(links: unknown[] = []) {
    const object = { id: "1", title: "Original", [linkField]: links };
    const fn = () =>
      kind === "scene"
        ? SceneEditPanel({
            scene: object,
            isVisible: false,
            onSubmit: page.onSubmit,
          })
        : GalleryEditPanel({
            gallery: object,
            isVisible: false,
            onSubmit: page.onSubmit,
            onDelete: vi.fn(),
          });
    render(fn);
    return () => render(fn);
  }
  function copyMenu(tree: Element[]) {
    return tree.find(
      (e) =>
        e.type === Dropdown &&
        elements(e.props.children as ReactNode).some(
          (c) =>
            c.props.id === `actions.copy_from_linked_${other.toLowerCase()}`
        )
    );
  }
  async function open() {
    page.query.mockResolvedValue({ data: { [`find${other}`]: source } });
    const draw = panel([
      { id: "23", title: "First" },
      { id: "24", title: "Second" },
    ]);
    const menu = copyMenu(draw());
    expect(menu).toBeDefined();
    const items = elements(menu!.props.children as ReactNode).filter(
      (e) => e.type === Dropdown.Item
    );
    expect(items.map((e) => e.props.children)).toEqual(["First", "Second"]);
    await (items[1].props.onClick as () => Promise<void>)();
    expect(page.query).toHaveBeenCalledWith(
      expect.objectContaining({
        query:
          kind === "scene" ? GQL.FindGalleryDocument : GQL.FindSceneDocument,
        variables: { id: "24" },
        fetchPolicy: "network-only",
      })
    );
    const dialog = draw().find((e) =>
      kind === "scene"
        ? e.type === "lazy-dialog" && !!e.props.scraped
        : e.type === GalleryScrapeDialog
    );
    expect(dialog).toBeDefined();
    return dialog!;
  }
  function renderDialog(dialog: Element) {
    hooks.states = [];
    hooks.dependencies = [];
    return render(() =>
      kind === "scene"
        ? SceneScrapeDialog(
            dialog.props as unknown as ComponentProps<typeof SceneScrapeDialog>
          )
        : GalleryScrapeDialog(
            dialog.props as unknown as ComponentProps<
              typeof GalleryScrapeDialog
            >
          )
    );
  }
  it("shows choices for linked objects and hides the action without links", async () => {
    await open();
    hooks.states = [];
    hooks.dependencies = [];
    page.values = undefined;
    expect(copyMenu(panel()())).toBeUndefined();
    const draw = panel([{ id: "23", title: "First" }]);
    const selector = draw().find(
      (e) =>
        Array.isArray(e.props.values) &&
        e.props.values.length === 1 &&
        typeof e.props.onSelect === "function"
    );
    expect(copyMenu(draw())).toBeDefined();
    (selector!.props.onSelect as (items: unknown[]) => void)([]);
    expect(copyMenu(draw())).toBeUndefined();
  });
  it("opens the scrape dialog with shared fields and existing object IDs", async () => {
    const dialog = await open();
    expect(dialog.props.scraped).toEqual(
      expect.objectContaining({
        title: source.title,
        code: source.code,
        date: source.date,
        details: source.details,
        urls: source.urls,
        [kind === "scene" ? "director" : "photographer"]:
          kind === "scene" ? source.photographer : source.director,
        studio: { stored_id: "4", name: "Existing studio" },
        performers: [{ stored_id: "5", name: "Existing performer" }],
        tags: [{ stored_id: "6", name: "Existing tag" }],
      })
    );
    if (kind === "scene") {
      expect(page.image).toHaveBeenCalledWith(source.paths.cover);
      expect((dialog.props.scraped as GQL.ScrapedScene).image).toBe(
        "data:image/jpeg;base64,cover"
      );
    }
    expect(page.values!.title).toBe("Original");
    expect(page.onSubmit).not.toHaveBeenCalled();
    const rows = renderDialog(dialog).filter((e) =>
      ["studio", "performers", "tags"].includes(e.props.field as string)
    );
    expect(rows).toHaveLength(3);
    for (const row of rows) {
      expect(row.props.newObjects ?? []).toEqual([]);
      expect(row.props.newStudio).toBeUndefined();
      const result = row.props.result as { newValue: unknown };
      expect(result.newValue).toEqual(
        row.props.field === "studio"
          ? { stored_id: "4", name: "Existing studio" }
          : [
              {
                stored_id: row.props.field === "performers" ? "5" : "6",
                name:
                  row.props.field === "performers"
                    ? "Existing performer"
                    : "Existing tag",
              },
            ]
      );
    }
  });
  it("applies only to the form and persists only on Save", async () => {
    const dialog = await open();
    const [scrapeDialog] = renderDialog(dialog);
    (scrapeDialog.props.onClose as (apply: boolean) => void)(true);
    expect(page.values).toMatchObject({
      title: "Linked title",
      studio_id: "4",
      performer_ids: ["5"],
      tag_ids: ["6"],
    });
    expect(page.onSubmit).not.toHaveBeenCalled();
    page.submit!();
    expect(page.onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Linked title",
        studio_id: "4",
        performer_ids: ["5"],
        tag_ids: ["6"],
      }),
      undefined
    );
  });
  it("leaves the form untouched when cancelled", async () => {
    const dialog = await open();
    const [scrapeDialog] = renderDialog(dialog);
    (scrapeDialog.props.onClose as (apply: boolean) => void)(false);
    expect(page.values!.title).toBe("Original");
    expect(page.onSubmit).not.toHaveBeenCalled();
  });
});
