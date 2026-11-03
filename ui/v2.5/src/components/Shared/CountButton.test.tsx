import { readFileSync } from "node:fs";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import { CountButton } from "./CountButton";

// CountButton uses no hooks, so calling it gives its element tree and the
// buttons' click handlers can be run directly.
type ButtonProps = { onClick: () => void; title?: string };
function buttons(props: React.ComponentProps<typeof CountButton>) {
  const group = CountButton(props) as React.ReactElement<{
    children: React.ReactElement<ButtonProps>[];
  }>;
  const [icon, count] = group.props.children.filter(React.isValidElement);
  return { icon: icon.props, count: count.props };
}

describe("play count button (#1970)", () => {
  it("with nothing to add, the icon opens the history like the count does", () => {
    const onValueClicked = vi.fn();
    const { icon, count } = buttons({
      value: 3,
      icon: null,
      onValueClicked,
      title: "Play count",
      countTitle: "View history",
    });
    icon.onClick();
    count.onClick();
    expect(onValueClicked).toHaveBeenCalledTimes(2);
    expect(icon.title).toBe("Play count · View history");
  });

  it("still adds one when it has something to add (O-counter)", () => {
    const onIncrement = vi.fn();
    const { icon } = buttons({ value: 3, icon: null, onIncrement, title: "O" });
    icon.onClick();
    expect(onIncrement).toHaveBeenCalledTimes(1);
    expect(icon.title).toBe("O");
  });

  it.each([
    ["../Scenes/SceneDetails/Scene.tsx", "scene-history-panel"],
    ["../Audios/AudioDetails/Audio.tsx", "audio-history-panel"],
  ])("%s: tapping it opens History and never adds a play", (file, tab) => {
    const src = readFileSync(new URL(file, import.meta.url), "utf8");
    const button = src.slice(src.indexOf("<ViewCountButton"));
    const props = button.slice(0, button.indexOf("/>"));
    expect(props).not.toContain("onIncrement");
    expect(props).toContain(`setActiveTabKey("${tab}")`);
  });
});
