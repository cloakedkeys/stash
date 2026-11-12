import type { ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import type * as GQL from "src/core/generated-graphql";
import type { GridCard } from "../Shared/GridCard/GridCard";
import { SceneCardGrid } from "./SceneCardGrid";

const cards = vi.hoisted(() => [] as ComponentProps<typeof GridCard>[]);
vi.mock("../Shared/GridCard/GridCard", () => ({
  useContainerDimensions: () => [undefined, { width: 1000 }],
  useCardWidth: () => 280,
  GridCard: (props: ComponentProps<typeof GridCard>) => {
    cards.push(props);
    return null;
  },
}));
vi.mock("../Shared/HoverScrubber", () => ({ HoverScrubber: () => null }));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
}));
vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

it("uses the existing card drag handles for direct members, excluding descendant scenes", () => {
  const move = vi.fn();
  renderToStaticMarkup(
    <SceneCardGrid
      scenes={
        ["10", "20"].map((id) => ({
          id,
          title: "Scene",
          files: [],
          groups: [{ group: { id } }],
        })) as unknown as GQL.SlimSceneDataFragment[]
      }
      selectedIds={new Set()}
      zoomIndex={1}
      onSelectChange={() => {}}
      fromGroupId="10"
      onMove={move}
    />
  );
  expect(cards[0].objectId).toBe("10");
  expect(cards[0].onMove).toBe(move);
  expect(cards[1].onMove).toBeUndefined();
});
