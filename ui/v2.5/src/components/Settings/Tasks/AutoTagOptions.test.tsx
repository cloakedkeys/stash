import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, expect, it, vi } from "vitest";
import type { AutoTagMetadataInput } from "src/core/generated-graphql";
import { AutoTagOptions } from "./AutoTagOptions";

const state = vi.hoisted(() => ({
  settings: [] as {
    id: string;
    checked: boolean;
    headingID: string;
    subHeadingID?: string;
    onChange: (v: boolean) => void;
  }[],
}));
vi.mock("../Inputs", () => ({
  BooleanSetting: (props: (typeof state.settings)[number]) => {
    state.settings.push(props);
    return (
      <input type="checkbox" id={props.id} checked={props.checked} readOnly />
    );
  },
}));
beforeEach(() => {
  state.settings = [];
});

it.each([
  undefined,
  false,
  true,
])("shows the saved longest match option (%s), defaulting to off", (saved) => {
  renderToStaticMarkup(
    <AutoTagOptions
      options={{ longest_match_only: saved }}
      setOptions={vi.fn()}
    />
  );
  const checkbox = state.settings.find(
    (s) => s.id === "autotag-longest-match-only"
  );
  expect(checkbox?.checked).toBe(saved ?? false);
  expect(checkbox?.headingID).toBe(
    "config.tasks.auto_tag.longest_match_only.heading"
  );
  expect(checkbox?.subHeadingID).toBe(
    "config.tasks.auto_tag.longest_match_only.description"
  );
});

it("passes the toggled option with the other defaults for saving and running", () => {
  const options: AutoTagMetadataInput = {
    performers: ["*"],
    studios: [],
    tags: ["*"],
    longest_match_only: false,
  };
  const setOptions = vi.fn();
  renderToStaticMarkup(
    <AutoTagOptions options={options} setOptions={setOptions} />
  );
  state.settings
    .find((s) => s.id === "autotag-longest-match-only")
    ?.onChange(true);
  expect(setOptions).toHaveBeenCalledWith({
    ...options,
    longest_match_only: true,
  });
});
