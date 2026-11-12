import React, { isValidElement, ReactElement, ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ScrapeResult,
  ObjectListScrapeResult,
  mergeLists,
} from "./scrapeResult";
import { ScrapeDialog, ScrapeDialogContext } from "./ScrapeDialog";
import {
  ScrapeDialogRow,
  ScrapedStringListRow,
  ScrapedTextAreaRow,
} from "./ScrapeDialogRow";
import { ScrapedObjectsRow, ScrapedPerformersRow } from "./ScrapedObjectsRow";
import { SceneScrapeDialog } from "src/components/Scenes/SceneDetails/SceneScrapeDialog";
import { StudioScrapeDialog } from "src/components/Studios/StudioDetails/StudioScrapeDialog";
import type { IUIConfig } from "src/core/config";

const hooks = vi.hoisted(() => ({
  context: {} as Record<string, unknown>,
  states: [] as unknown[],
  index: 0,
  refs: [] as { current: unknown }[],
  refIndex: 0,
  effects: [] as (() => void)[],
  ui: {} as IUIConfig,
  save: vi.fn(() => Promise.resolve()),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof React>()),
  useContext: () => hooks.context,
  useMemo: (f: () => unknown) => f(),
  useRef: (value: unknown) => {
    const index = hooks.refIndex++;
    hooks.refs[index] ??= { current: value };
    return hooks.refs[index];
  },
  useEffect: (f: () => void) => {
    hooks.effects.push(f);
  },
  useState: (value: unknown) => {
    const index = hooks.index++;
    if (!(index in hooks.states)) hooks.states[index] = value;
    return [
      hooks.states[index],
      (next: unknown) => {
        hooks.states[index] = next;
      },
    ];
  },
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
  FormattedMessage: "message",
}));
vi.mock("src/core/StashService", () => ({
  useConfigureUISetting: () => [hooks.save],
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({
    configuration: { ui: hooks.ui, interface: {} },
  }),
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({ error: vi.fn() }) }));
vi.mock("../CountrySelect", () => ({ CountrySelect: "select" }));
vi.mock("../ImageSelector", () => ({ ImageSelector: "img" }));
vi.mock("src/components/Performers/PerformerSelect", () => ({
  PerformerSelect: "performers",
}));
vi.mock("src/components/Groups/GroupSelect", () => ({ GroupSelect: "groups" }));
vi.mock("src/components/Studios/StudioSelect", () => ({
  StudioSelect: "studios",
}));
vi.mock("src/components/Tags/TagSelect", () => ({ TagIDSelect: "tags" }));
vi.mock("./createObjects", () => ({
  useCreateScrapedPerformer: () => {},
  useCreateScrapedGroup: () => {},
  useCreateScrapedStudio: () => {},
}));
vi.mock("./scrapedTags", () => ({
  useScrapedTags: () => ({ tags: new ScrapeResult(), newTags: [] }),
}));
vi.mock("src/patch", () => ({
  PatchComponent: (_: string, component: unknown) => component,
}));
vi.mock("src/utils/scene", () => ({ sceneAgeFromDate: () => undefined }));

// Inspect React elements and invoke the same handlers as the browser. Hooks are
// kept in memory because this project has no DOM test environment dependency.
type Element = ReactElement<Record<string, unknown>>;
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements);
  if (!isValidElement<Record<string, unknown>>(node)) return [];
  return [node, ...elements(node.props.children as ReactNode)];
}
function render<T>(f: () => T) {
  hooks.index = 0;
  hooks.refIndex = 0;
  const ret = f();
  for (const effect of hooks.effects.splice(0)) effect();
  return ret;
}
function click(node: ReactNode, merge = false, index = 0) {
  const buttons = elements(node).filter(
    (e) =>
      typeof e.props.onClick === "function" &&
      (merge ? "aria-pressed" in e.props : !("aria-pressed" in e.props))
  );
  expect(
    buttons[index],
    merge ? "Merge choice" : "Existing/scraped choice"
  ).toBeDefined();
  (buttons[index].props.onClick as () => void)();
}
function message(node: ReactNode, id: string) {
  return elements(node).some((e) => e.props.id === id);
}

beforeEach(() => {
  hooks.context = { mergeDefaults: {} };
  hooks.states = [];
  hooks.refs = [];
  hooks.effects = [];
  hooks.ui = {};
  hooks.save.mockClear();
});

describe("list selection", () => {
  it("merges current first, deduplicates by stored ID and saves that selection", () => {
    let result = new ObjectListScrapeResult(
      [{ stored_id: "1", name: "Current" }],
      [
        { stored_id: "1", name: "Remote name" },
        { stored_id: "2", name: "New" },
        { stored_id: "2", name: "Repeat" },
      ]
    );
    const row = () => {
      const shared = ScrapedObjectsRow({
        title: "Performers",
        field: "performers",
        result,
        onChange: (value) => {
          result = value;
        },
        renderObjects: (value) => (
          <div>{value.originalValue?.map((p) => p.name).join(", ")}</div>
        ),
        getName: (p) => p.name,
      });
      return render(() => ScrapeDialogRow(shared.props));
    };
    const scraped = result.newValue;
    click(row(), true);
    expect(result.getNewValue()).toEqual([
      { stored_id: "1", name: "Current" },
      { stored_id: "2", name: "New" },
    ]);
    expect(
      elements(row()).some((e) => e.props.children === "Current, New")
    ).toBe(true);
    click(row(), false, 0);
    expect(result.currentValue()).toEqual(result.originalValue);
    expect(result.getNewValue()).toBeUndefined();
    click(row(), false, 1);
    expect(result.getNewValue()).toBe(scraped);
  });

  it("merges aliases case-insensitively without changing either old choice", () => {
    let result = new ScrapeResult(
      ["Alice", "ALICE", "Beth"],
      ["alice", "Cara", "CARA"]
    );
    const shared = () =>
      ScrapedStringListRow({
        title: "Aliases",
        field: "aliases",
        result,
        onChange: (value) => {
          result = value;
        },
      }) as Element;
    const row = () =>
      render(() =>
        ScrapeDialogRow(
          shared().props as unknown as Parameters<
            typeof ScrapeDialogRow<string[]>
          >[0]
        )
      );
    expect(result.getNewValue()).toEqual(["alice", "Cara", "CARA"]);
    click(row(), true);
    expect(result.getNewValue()).toEqual(["Alice", "Beth", "Cara"]);
    click(row(), false, 1);
    expect(result.getNewValue()).toEqual(["alice", "Cara", "CARA"]);
  });

  it("keeps merge selected when scraped values are edited or newly linked", () => {
    const result = new ScrapeResult(["current"], ["scraped"]).cloneWithMerge(
      mergeLists
    );
    expect(result.cloneWithValue(["linked"]).getNewValue()).toEqual([
      "current",
      "linked",
    ]);
    expect(result.originalValue).toEqual(["current"]);
    expect(result.newValue).toEqual(["scraped"]);
  });

  it("handles empty lists and keeps the existing default", () => {
    expect(mergeLists(undefined, ["new", "new"])).toEqual(["new"]);
    expect(mergeLists(["old"], undefined)).toEqual(["old"]);
    const result = new ScrapeResult(["old"], ["new"]);
    expect(result.useMergedValue).toBe(false);
    expect(result.getNewValue()).toEqual(["new"]);
  });
});

it("does not add merging to automatic unions, scalar fields or object merge dialogs", () => {
  for (const props of [
    { field: "urls" },
    { field: "aliases", mergeable: false },
  ]) {
    const shared = ScrapedStringListRow({
      ...props,
      title: "List",
      result: new ScrapeResult(["old"], ["new"]),
      onChange: () => {},
    }) as Element;
    expect(
      message(
        render(() =>
          ScrapeDialogRow(
            shared.props as unknown as Parameters<
              typeof ScrapeDialogRow<string[]>
            >[0]
          )
        ),
        "actions.merge"
      )
    ).toBe(false);
  }
  const shared = ScrapedTextAreaRow({
    field: "details",
    title: "Details",
    result: new ScrapeResult("old", "new"),
    onChange: () => {},
  }) as Element;
  expect(
    message(
      render(() =>
        ScrapeDialogRow(
          shared.props as unknown as Parameters<
            typeof ScrapeDialogRow<string>
          >[0]
        )
      ),
      "actions.merge"
    )
  ).toBe(false);
  hooks.context = {};
  expect(
    message(
      render(() =>
        ScrapeDialogRow({
          field: "tags",
          title: "Tags",
          result: new ScrapeResult(["old"], ["new"]),
          mergeValues: mergeLists,
          originalField: null,
          newField: null,
          onChange: () => {},
        })
      ),
      "actions.merge"
    )
  ).toBe(false);
});

it("uses an opted-in default only on opening, so keep and replace still work", () => {
  let result = new ScrapeResult(["old"], ["new"]);
  hooks.context = { mergeDefaults: { tags: true } };
  const row = () =>
    render(() =>
      ScrapeDialogRow({
        field: "tags",
        title: "Tags",
        result,
        mergeValues: mergeLists,
        originalField: null,
        newField: null,
        onChange: (value) => {
          result = value;
        },
      })
    );
  row();
  expect(result.getNewValue()).toEqual(["old", "new"]);
  click(row(), false, 0);
  row();
  expect(result.getNewValue()).toBeUndefined();
});

it("applies the remembered default when a missing object is linked later", () => {
  let result = new ScrapeResult<string[]>(["current"]);
  hooks.context = { mergeDefaults: { tags: true } };
  const row = () =>
    render(() =>
      ScrapeDialogRow({
        field: "tags",
        title: "Tags",
        result,
        mergeValues: mergeLists,
        originalField: null,
        newField: null,
        newValues: <div>Missing tag</div>,
        onChange: (value) => {
          result = value;
        },
      })
    );
  row();
  result = result.cloneWithValue(["linked"]);
  row();
  expect(result.getNewValue()).toEqual(["current", "linked"]);
});

it("stores a per-object, per-field default through UIConfig, preserving other defaults", async () => {
  hooks.ui.scrapeMergeDefaults = {
    performer: { aliases: true },
    scene: { tags: true },
  };
  const dialog = render(() =>
    ScrapeDialog({ scrapeType: "scene", title: "Scrape", onClose: () => {} })
  );
  const provider = elements(dialog).find(
    (e) => e.type === ScrapeDialogContext.Provider
  )!;
  const context = provider.props.value as React.ContextType<
    typeof ScrapeDialogContext
  >;
  expect(context.mergeDefaults).toEqual({ tags: true });
  context.setMergeDefault!("performers", true);
  expect(hooks.save).toHaveBeenCalledWith({
    variables: {
      key: "scrapeMergeDefaults",
      value: {
        performer: { aliases: true },
        scene: { tags: true, performers: true },
      },
    },
  });
  context.setMergeDefault!("tags", false);
  expect(hooks.save).toHaveBeenLastCalledWith({
    variables: {
      key: "scrapeMergeDefaults",
      value: { performer: { aliases: true }, scene: { tags: false } },
    },
  });
});

it("Apply returns merged performers from the real scene dialog", () => {
  const saved = vi.fn();
  const props = {
    scene: { title: "Scene" },
    sceneStudio: null,
    scenePerformers: [{ id: "1", name: "Current", alias_list: [] }],
    sceneTags: [],
    sceneGroups: [],
    scraped: { performers: [{ stored_id: "2", name: "New" }] },
    onClose: saved,
  };
  const dialog = () => render(() => SceneScrapeDialog(props)) as Element;
  const performers = elements(dialog()).find(
    (e) => e.type === ScrapedPerformersRow
  )!;
  const objects = ScrapedPerformersRow(
    performers.props as Parameters<typeof ScrapedPerformersRow>[0]
  ) as Element;
  const shared = ScrapedObjectsRow(
    objects.props as unknown as Parameters<typeof ScrapedObjectsRow>[0]
  );
  // The row has its own hooks, independently of the containing dialog.
  hooks.refs = [];
  click(
    render(() => ScrapeDialogRow(shared.props)),
    true
  );
  (dialog().props.onClose as (apply: boolean) => void)(true);
  expect(saved.mock.calls[0][0].performers).toEqual([
    { stored_id: "1", name: "Current" },
    { stored_id: "2", name: "New" },
  ]);
});

it("Apply returns merged studio aliases", () => {
  const saved = vi.fn();
  const props = {
    studio: { name: "Studio", aliases: ["Alice", "Beth"] },
    studioTags: [],
    scraped: { name: "Studio", aliases: "alice, Cara, CARA" },
    scraper: {
      endpoint: "https://example.com/graphql",
      name: "Fixture",
      index: 0,
      api_key: "",
      max_requests_per_minute: 0,
    },
    onClose: saved,
  };
  const dialog = () => render(() => StudioScrapeDialog(props)) as Element;
  const alias = elements(dialog()).find((e) => e.props.field === "aliases")!;
  const shared = ScrapedTextAreaRow(
    alias.props as unknown as Parameters<typeof ScrapedTextAreaRow>[0]
  ) as Element;
  hooks.refs = [];
  click(
    render(() =>
      ScrapeDialogRow(
        shared.props as unknown as Parameters<typeof ScrapeDialogRow<string>>[0]
      )
    ),
    true
  );
  (dialog().props.onClose as (apply: boolean) => void)(true);
  expect(saved.mock.calls[0][0].aliases).toBe("Alice, Beth, Cara");
});
