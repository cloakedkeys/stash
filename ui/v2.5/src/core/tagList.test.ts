import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { buildSchema, execute } from "graphql";
import { describe, expect, it, vi } from "vitest";
import { Children, ReactElement } from "react";
import "src/components/Tags/TagCard";
import { components } from "src/patch";
import * as GQL from "./generated-graphql";
import { useFindTagsForList } from "./StashService";
import { ListFilterModel } from "src/models/list-filter/filter";
import NavUtils from "src/utils/navigation";

const config = vi.hoisted(() => ({
  showChildTagContent: undefined as boolean | undefined,
}));

vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { ui: config } }),
}));
vi.mock("./createClient", () => ({ createClient: () => ({}) }));
vi.mock("./generated-graphql", async (importOriginal) => ({
  ...(await importOriginal<typeof GQL>()),
  useFindTagsForListQuery: vi.fn(),
}));

function readSchema(directory: string): string {
  return readdirSync(directory, { withFileTypes: true })
    .map((entry) => {
      const path = resolve(directory, entry.name);
      if (entry.isDirectory()) return readSchema(path);
      return entry.name.endsWith(".graphql") ? readFileSync(path, "utf8") : "";
    })
    .join("\n");
}

const schema = buildSchema(readSchema(resolve("../../graphql/schema")));

describe("tag list scene counts", () => {
  it.each([
    { fragment: "TagData", setting: true, count: 2 },
    { fragment: "TagData", setting: false, count: 1 },
    { fragment: "TagListData", setting: true, count: 2 },
    { fragment: "TagListData", setting: false, count: 1 },
  ])("shows $count scenes on $fragment cards when subtag content is $setting", ({
    fragment,
    setting,
    count,
  }) => {
    config.showChildTagContent = setting;
    const tag = {
      id: "parent",
      name: "Parent",
      ...(fragment === "TagData"
        ? { scene_count: 1, scene_count_all: 2 }
        : { scene_count: setting ? 2 : 1 }),
    };
    const popovers = components["TagCard.Popovers"]({ tag }) as ReactElement;
    const group = Children.toArray(popovers.props.children)[1] as ReactElement;
    const sceneButton = Children.toArray(
      group.props.children
    )[0] as ReactElement;

    expect(sceneButton.props.count).toBe(count);
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    filter.configureFromQueryString(sceneButton.props.url.split("?")[1]);
    expect(filter.makeFilter()).toMatchObject({
      tags: { value: ["parent"], depth: setting ? -1 : 0 },
    });
  });

  it.each([
    { setting: true, depth: -1 },
    { setting: false, depth: 0 },
    { setting: undefined, depth: 0 },
  ])("links to scenes at depth $depth when subtag content is $setting", ({
    setting,
    depth,
  }) => {
    const url = NavUtils.makeTagScenesUrl(
      { id: "parent", name: "Parent" },
      setting ? -1 : 0
    );
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    filter.configureFromQueryString(url.split("?")[1]);

    expect(filter.makeFilter()).toMatchObject({
      tags: { value: ["parent"], depth },
    });
  });

  it("keeps scene links direct when depth is omitted", () => {
    const url = NavUtils.makeTagScenesUrl({ id: "parent", name: "Parent" });
    const filter = new ListFilterModel(GQL.FilterMode.Scenes);
    filter.configureFromQueryString(url.split("?")[1]);

    expect(filter.makeFilter()).toMatchObject({
      tags: { value: ["parent"], depth: 0 },
    });
  });

  it.each([
    { setting: true, count: 2 },
    { setting: false, count: 1 },
    { setting: undefined, count: 1 },
  ])("returns $count scenes when subtag content is $setting", async ({
    setting,
    count,
  }) => {
    config.showChildTagContent = setting;
    useFindTagsForList(new ListFilterModel(GQL.FilterMode.Tags));
    const options = vi.mocked(GQL.useFindTagsForListQuery).mock.lastCall?.[0];

    // Model the server's existing depth-aware resolver: one scene on the
    // parent, with a second scene available through its child.
    const parentScenes = ["scene1"];
    const childScenes = ["scene1", "scene2"];
    const result = await execute({
      schema,
      document: GQL.FindTagsForListDocument,
      variableValues: options?.variables,
      rootValue: {
        findTags: {
          count: 1,
          tags: [
            {
              id: "parent",
              name: "Parent",
              aliases: [],
              ignore_auto_tag: false,
              favorite: false,
              stash_ids: [],
              image_path: "",
              parents: [],
              children: [],
              scene_count: ({ depth }: { depth?: number }) =>
                new Set(
                  depth === -1
                    ? [...parentScenes, ...childScenes]
                    : parentScenes
                ).size,
              audio_count: 0,
              scene_marker_count: 0,
              image_count: 0,
              gallery_count: 0,
              performer_count: 0,
              studio_count: 0,
              group_count: 0,
            },
          ],
        },
      },
    });

    expect(result.errors).toBeUndefined();
    expect(result.data).toMatchObject({
      findTags: { tags: [{ scene_count: count }] },
    });
  });
});
