import React, {
  isValidElement,
  ReactElement,
  ReactNode,
  SetStateAction,
} from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { NewScrapedObjects, ScrapedObjectsRow } from "./ScrapedObjectsRow";
import {
  useCreateScrapedTag,
  useCreateScrapedPerformer,
  useCreateScrapedGroup,
} from "./createObjects";
import { ScrapeResult, mergeLists } from "./scrapeResult";
import { scrapedPerformerToCreateInput } from "src/core/performers";
import { scrapedGroupToCreateInput } from "src/core/groups";

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof React>()),
  useState: (value: unknown) => [value, vi.fn()],
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("src/core/StashService", () => ({
  useTagCreate: () => [mocks.create],
  usePerformerCreate: () => [mocks.create],
  useGroupCreate: () => [mocks.create],
  useStudioCreate: () => [mocks.create],
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => mocks }));
vi.mock("src/components/Performers/PerformerSelect", () => ({
  PerformerSelect: "performers",
}));
vi.mock("src/components/Groups/GroupSelect", () => ({ GroupSelect: "groups" }));
vi.mock("src/components/Studios/StudioSelect", () => ({
  StudioSelect: "studios",
}));
vi.mock("src/components/Tags/TagSelect", () => ({ TagIDSelect: "tags" }));
vi.mock("../CountrySelect", () => ({ CountrySelect: "select" }));
vi.mock("../ImageSelector", () => ({ ImageSelector: "img" }));

type Item = {
  name: string;
  stored_id?: string;
  remote_site_id?: string;
  urls?: string[];
  image?: string;
  synopsis?: string;
};
type Element = ReactElement<Record<string, unknown>>;
function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements);
  if (!isValidElement<Record<string, unknown>>(node)) return [];
  return [node, ...elements(node.props.children as ReactNode)];
}
const endpoint = "https://example.com/graphql";
const cases = [
  {
    field: "tags",
    hook: useCreateScrapedTag,
    mutation: "tagCreate",
    input: (p: Item) => ({
      name: p.name,
      stash_ids: [{ endpoint, stash_id: p.remote_site_id }],
    }),
  },
  {
    field: "performers",
    hook: useCreateScrapedPerformer,
    mutation: "performerCreate",
    input: (p: Item) => scrapedPerformerToCreateInput(p, endpoint),
  },
  {
    field: "groups",
    hook: useCreateScrapedGroup,
    mutation: "groupCreate",
    input: (p: Item) => scrapedGroupToCreateInput(p),
  },
];
function setup(testCase: (typeof cases)[number], merge = false, count = 3) {
  let missing: Item[] = Array.from({ length: count }, (_, i) => ({
    name: `New ${i}`,
    remote_site_id: `remote-${i}`,
    urls: [`https://example.com/${i}`],
    image: "image",
    synopsis: "Synopsis",
  }));
  const original = [{ name: "Current", stored_id: "current" }];
  const matched = [{ name: "Matched", stored_id: "matched" }];
  let result = new ScrapeResult<Item[]>(original, matched);
  if (merge)
    result = result.cloneWithMerge((a, b) =>
      mergeLists(a, b, (p) => p.stored_id)
    );
  mocks.create.mockImplementation(
    async ({ variables }: { variables: { input: Item } }) => ({
      data: {
        [testCase.mutation]: {
          id: `id-${variables.input.name}`,
          name: variables.input.name,
        },
      },
    })
  );
  // All fixtures have a name and use the common scraped-object fields.
  const createFromHook = testCase.hook as unknown as (props: {
    scrapeResult: ScrapeResult<Item[]>;
    newObjects: Item[];
    setScrapeResult: React.Dispatch<SetStateAction<ScrapeResult<Item[]>>>;
    setNewObjects: React.Dispatch<SetStateAction<Item[]>>;
    endpoint: string;
  }) => (items: Item | Item[]) => Promise<void>;
  const create = createFromHook({
    scrapeResult: result,
    newObjects: missing,
    setScrapeResult: (next: SetStateAction<ScrapeResult<Item[]>>) => {
      result = typeof next === "function" ? next(result) : next;
    },
    setNewObjects: (next: SetStateAction<Item[]>) => {
      missing = typeof next === "function" ? next(missing) : next;
    },
    endpoint,
  });
  const link = vi.fn();
  const row = () =>
    ScrapedObjectsRow<Item>({
      title: testCase.field,
      field: testCase.field,
      result,
      onChange: (value) => {
        result = value;
      },
      newObjects: missing,
      onCreateNew: create,
      onLinkExisting: link,
      renderObjects: () => <div />,
      getName: (p) => p.name,
    });
  const buttons = () => {
    const newValues = row().props.newValues;
    expect(isValidElement(newValues)).toBe(true);
    return NewScrapedObjects(
      (newValues as ReactElement<Parameters<typeof NewScrapedObjects<Item>>[0]>)
        .props
    );
  };
  const all = async () => {
    const button = elements(buttons()).find(
      (e) => e.props.children === "actions.create_all"
    );
    expect(button, "Create all button").toBeDefined();
    await (button!.props.onClick as () => Promise<void>)();
  };
  return {
    all,
    buttons,
    link,
    row,
    result: () => result,
    missing: () => missing,
  };
}
beforeEach(() => vi.clearAllMocks());

for (const testCase of cases) {
  for (const merge of [false, true]) {
    it(`creates every missing ${testCase.field}, preserving selection (Merge=${merge})`, async () => {
      // Ten also exercises the collapsed missing-items list.
      const state = setup(testCase, merge, 10);
      const missing = [...state.missing()];
      await state.all();
      expect(mocks.create).toHaveBeenCalledTimes(10);
      missing.forEach((item, i) => {
        expect(mocks.create).toHaveBeenNthCalledWith(i + 1, {
          variables: { input: testCase.input(item) },
        });
      });
      expect(state.missing()).toEqual([]);
      expect(state.result().newValue).toEqual([
        { name: "Matched", stored_id: "matched" },
        ...missing.map((p) => ({ name: p.name, stored_id: `id-${p.name}` })),
      ]);
      expect(state.result().useMergedValue).toBe(merge);
      expect(state.result().getNewValue()).toEqual([
        ...(merge ? [{ name: "Current", stored_id: "current" }] : []),
        ...state.result().newValue!,
      ]);
      expect(state.row().props.newValues).toBeUndefined();
      expect(mocks.error).not.toHaveBeenCalled();
    });
  }
  it(`keeps earlier ${testCase.field} after a failure and allows retry`, async () => {
    const state = setup(testCase, true);
    const error = new Error("Create failed");
    // First item succeeds, second fails; the third is left for a retry.
    mocks.create.mockReset();
    mocks.create
      .mockResolvedValueOnce({
        data: { [testCase.mutation]: { id: "first", name: "New 0" } },
      })
      .mockRejectedValueOnce(error);
    await state.all();
    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.error).toHaveBeenCalledWith(error);
    expect(state.result().getNewValue()).toEqual([
      { name: "Current", stored_id: "current" },
      { name: "Matched", stored_id: "matched" },
      { name: "New 0", stored_id: "first" },
    ]);
    expect(state.missing().map((p) => p.name)).toEqual(["New 1", "New 2"]);
    mocks.create.mockImplementation(
      async ({ variables }: { variables: { input: Item } }) => ({
        data: {
          [testCase.mutation]: {
            id: variables.input.name,
            name: variables.input.name,
          },
        },
      })
    );
    await state.all();
    expect(state.missing()).toEqual([]);
    expect(state.result().newValue).toHaveLength(4);
  });
  it(`preserves per-item create and link for ${testCase.field}`, async () => {
    const state = setup(testCase);
    const item = state.missing()[0];
    const badge = elements(state.buttons()).find(
      (e) => e.props.variant === "secondary" && e.props.className === "tag-item"
    )!;
    await (badge.props.onClick as () => Promise<void>)();
    expect(mocks.create).toHaveBeenCalledExactlyOnceWith({
      variables: { input: testCase.input(item) },
    });
    expect(state.result().newValue).toHaveLength(2);
    expect(state.missing()).toHaveLength(2);
    const link = elements(badge).find((e) => e.props.className === "minimal")!;
    const stopPropagation = vi.fn();
    (link.props.onClick as (e: { stopPropagation: () => void }) => void)({
      stopPropagation,
    });
    expect(state.link).toHaveBeenCalledExactlyOnceWith(item);
    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(mocks.create).toHaveBeenCalledTimes(1);
  });
}
