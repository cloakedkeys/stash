import type { ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { IntlProvider } from "react-intl";
import { expect, it, vi } from "vitest";
import type * as GQL from "src/core/generated-graphql";
import messages from "src/locales/en-GB.json";
import NavUtils from "src/utils/navigation";
import { SceneCard } from "./SceneCard";
import { SceneGroupPanel } from "./SceneDetails/SceneGroupPanel";
import type { GridCard } from "../Shared/GridCard/GridCard";

vi.mock("./PreviewScrubber", () => ({ PreviewScrubber: () => null }));
vi.mock("src/core/StashService", () => ({
  useGroupUserDataUpdate: () => [vi.fn()],
  useSceneUserDataUpdate: () => [vi.fn()],
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: undefined }),
}));
// Render the real card's content, without mounting previews or resize observers.
vi.mock("../Shared/GridCard/GridCard", () => ({
  GridCard: ({ popovers }: ComponentProps<typeof GridCard>) => (
    <div>{popovers}</div>
  ),
}));

function group(id = "10", scene_index: number | null = 2, scene_count = 5) {
  return {
    scene_index,
    group: {
      id,
      name: `Group ${id}`,
      scene_count,
      scenes: [],
      tags: [],
      containing_groups: [],
      sub_group_count: 0,
      organized: false,
    },
  };
}
function scene(groups: ReturnType<typeof group>[]) {
  return {
    id: "1",
    title: "A scene",
    files: [],
    groups,
    tags: [],
    galleries: [],
    performers: [],
    scene_markers: [],
  } as unknown as GQL.SceneDataFragment;
}
function render(groups: ReturnType<typeof group>[], page = false) {
  return renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      messages={messages as unknown as Record<string, string>}
    >
      <MemoryRouter>
        {page ? (
          <SceneGroupPanel scene={scene(groups)} />
        ) : (
          <SceneCard scene={scene(groups)} />
        )}
      </MemoryRouter>
    </IntlProvider>
  );
}

it("shows a linked part per numbered group on the scene page and leaves unnumbered groups alone", () => {
  const html = render([group(), group("20", 3, 8), group("30", null)], true);
  expect(html).toContain("Part 2 of 5");
  expect(html).toContain("Part 3 of 8");
  expect(html.match(/Part /g)).toHaveLength(2);
  for (const entry of [group(), group("20", 3, 8)]) {
    expect(html).toContain(
      `href="${NavUtils.makeGroupScenesUrl(entry.group).replaceAll("&", "&amp;")}"`
    );
  }
  expect(render([group("30", null)], true)).not.toContain("Part ");
  expect(render([], true)).not.toContain("Part ");
});

it("shows the card part for exactly one numbered group, including alongside unnumbered groups", () => {
  for (const groups of [[group()], [group(), group("20", null)]]) {
    const html = render(groups);
    expect(html).toContain("Part 2 of 5");
    expect(html).toContain(
      `href="${NavUtils.makeGroupScenesUrl(group().group).replaceAll("&", "&amp;")}"`
    );
    expect(html).toMatch(/group-count[\s\S]*Part 2 of 5/);
  }
  for (const groups of [[], [group("20", null)], [group(), group("20", 3)]]) {
    expect(render(groups)).not.toContain("Part ");
  }
});

it("only shows the total when the part number fits on the scene page and card", () => {
  for (const page of [true, false]) {
    const html = render([group("10", 7, 1)], page);
    expect(html).toContain("Part 7</a>");
    expect(html).not.toContain("of 1");
    expect(render([group("10", 2, 5)], page)).toContain("Part 2 of 5");
    expect(render([group("10", 1, 1)], page)).toContain("Part 1 of 1");
    expect(render([group("10", 0, 5)], page)).toContain("Part 0</a>");
    expect(render([group("10", -1, 5)], page)).toContain("Part -1</a>");
  }
});
