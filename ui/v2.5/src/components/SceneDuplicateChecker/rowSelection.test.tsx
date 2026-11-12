import React from "react";
import { Button, Dropdown, Form } from "react-bootstrap";
import { Link } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { DeleteScenesDialog } from "src/components/Scenes/DeleteScenesDialog";
import { SceneDuplicateChecker } from "./SceneDuplicateChecker";

const page = vi.hoisted(() => ({ state: [] as unknown[], next: 0 }));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useMemo: (calculate: () => unknown) => calculate(),
  useState: (initial: unknown) => {
    const index = page.next++;
    if (!(index in page.state)) page.state[index] = initial;
    return [page.state[index], (value: unknown) => (page.state[index] = value)];
  },
}));
vi.mock("react-intl", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-intl")>()),
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("react-router-dom", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-router-dom")>()),
  useHistory: () => ({ location: { search: "" }, push: vi.fn() }),
}));
vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
}));
vi.mock("src/core/generated-graphql", async (importOriginal) => ({
  ...(await importOriginal<typeof import("src/core/generated-graphql")>()),
  useFindDuplicateScenesQuery: () => ({
    data: { findDuplicateScenes: [scenes] },
    loading: false,
    refetch: vi.fn(),
  }),
  useFindScenesQuery: () => ({ data: undefined }),
}));

const scenes = ["h264", "hevc"].map((codec, index) => ({
  id: String(index + 1),
  title: `Scene ${index + 1}`,
  files: [
    {
      id: String(index + 1),
      path: `/scene${index + 1}.mp4`,
      size: (index + 1) * 100,
      video_codec: codec,
    },
  ],
  paths: {},
  tags: [],
  performers: [],
  groups: [],
  scene_markers: [],
  galleries: [],
})) as unknown as GQL.SlimSceneDataFragment[];

type Props = {
  children?: React.ReactNode;
  checked?: boolean;
  id?: string;
  to?: string;
  "data-action"?: string;
  selected?: GQL.SlimSceneDataFragment[];
  onClick?: (event: React.MouseEvent<HTMLTableRowElement>) => void;
  onChange?: (event: {
    currentTarget: { checked: boolean };
    target: { checked: boolean };
  }) => void;
};
type Element = React.ReactElement<Props>;

function elements(node: React.ReactNode): Element[] {
  return React.Children.toArray(node).flatMap((child) => {
    if (!React.isValidElement<Props>(child)) return [];
    return [child, ...elements(child.props.children)];
  });
}

function render() {
  page.next = 0;
  return elements(SceneDuplicateChecker({}));
}

function rows(tree = render()) {
  return tree.filter(
    (element) =>
      element.type === "tr" &&
      elements(element.props.children).some(
        (child) => child.type === Form.Check
      )
  );
}

function checkbox(row: Element) {
  return elements(row).find((element) => element.type === Form.Check)!;
}

// Model closest() for a target or a nested icon inside an interactive element.
function click(row: Element, ancestor?: string) {
  const target = {
    closest: (selector: string) =>
      ancestor &&
      selector
        .split(",")
        .map((part) => part.trim())
        .includes(ancestor)
        ? target
        : null,
  };
  row.props.onClick?.({
    target,
    currentTarget: row,
  } as unknown as React.MouseEvent<HTMLTableRowElement>);
}

function change(element: Element, checked: boolean) {
  element.props.onChange!({ currentTarget: { checked }, target: { checked } });
}

beforeEach(() => {
  page.state = [];
  page.next = 0;
  vi.stubGlobal("window", { getSelection: () => null });
});

afterEach(() => vi.unstubAllGlobals());

describe("duplicate scene row selection", () => {
  it.each([
    0, 1,
  ])("clicking row %i selects and deselects only that scene", (index) => {
    click(rows()[index]);
    expect(checkbox(rows()[index]).props.checked).toBe(true);
    expect(checkbox(rows()[1 - index]).props.checked).toBeFalsy();
    click(rows()[index]);
    expect(checkbox(rows()[index]).props.checked).toBe(false);
  });

  it.each([
    false,
    true,
  ])("preserves checked=%s when a click ends selecting text inside the row", (checked) => {
    if (checked) click(rows()[0]);
    const row = rows()[0];
    vi.stubGlobal("window", {
      getSelection: () => ({
        toString: () => "/scene1.mp4",
        containsNode: (node: unknown, partial: boolean) =>
          node === row && partial,
      }),
    });
    click(row);
    expect(!!checkbox(rows()[0]).props.checked).toBe(checked);
  });

  it.each([
    ["", true],
    ["/scene2.mp4", false],
  ])("still toggles with selection %j inside row=%s", (text, inside) => {
    vi.stubGlobal("window", {
      getSelection: () => ({
        toString: () => text,
        containsNode: () => inside,
      }),
    });
    click(rows()[0]);
    expect(checkbox(rows()[0]).props.checked).toBe(true);
  });

  it.each([
    "a",
    "button",
    "input",
    "select",
    "textarea",
    "label",
    '[role="button"]',
    '[contenteditable="true"]',
  ])("clicking inside %s preserves row selection", (ancestor) => {
    click(rows()[0]);
    expect(checkbox(rows()[0]).props.checked).toBe(true);
    click(rows()[0], ancestor);
    expect(checkbox(rows()[0]).props.checked).toBe(true);
  });

  it("keeps the title destination and delete button action", () => {
    click(rows()[0]);
    expect(checkbox(rows()[0]).props.checked).toBe(true);
    click(rows()[0]);
    const row = rows()[0];
    expect(
      elements(row).find((element) => element.type === Link)!.props.to
    ).toBe("/scenes/1");
    const button = elements(row).find(
      (element) =>
        element.type === Button && element.props["data-action"] === "delete"
    )!;
    button.props.onClick!({} as React.MouseEvent<HTMLTableRowElement>);
    click(row, "button");
    expect(checkbox(rows()[0]).props.checked).toBeFalsy();
    expect(
      render().find((element) => element.type === DeleteScenesDialog)!.props
        .selected
    ).toEqual([scenes[0]]);
  });

  it("the checkbox selects once and can undo a row selection", () => {
    click(rows()[0]);
    expect(checkbox(rows()[0]).props.checked).toBe(true);
    change(checkbox(rows()[0]), false);
    click(rows()[0], "input");
    expect(checkbox(rows()[0]).props.checked).toBe(false);
    change(checkbox(rows()[0]), true);
    click(rows()[0], "input");
    expect(checkbox(rows()[0]).props.checked).toBe(true);
  });

  it("keeps safe bulk selection and select none", () => {
    const selectLargest = () => {
      render().filter((element) => element.type === Dropdown.Item)[2].props
        .onClick!({} as React.MouseEvent<HTMLTableRowElement>);
    };
    selectLargest();
    expect(rows().map((row) => !!checkbox(row).props.checked)).toEqual([
      false,
      false,
    ]);
    change(
      render().find((element) => element.props.id === "chkSafeSelect")!,
      false
    );
    selectLargest();
    expect(rows().map((row) => !!checkbox(row).props.checked)).toEqual([
      true,
      false,
    ]);
    render().find((element) => element.type === Dropdown.Item)!.props.onClick!(
      {} as React.MouseEvent<HTMLTableRowElement>
    );
    expect(rows().map((row) => !!checkbox(row).props.checked)).toEqual([
      false,
      false,
    ]);
    click(rows()[1]);
    expect(checkbox(rows()[1]).props.checked).toBe(true);
    // Filter button, then the top pagination's edit and delete buttons.
    render().filter(
      (element) => element.type === Button && !element.props["data-action"]
    )[2].props.onClick!({} as React.MouseEvent<HTMLTableRowElement>);
    expect(
      render().find((element) => element.type === DeleteScenesDialog)!.props
        .selected
    ).toEqual([scenes[1]]);
  });
});
