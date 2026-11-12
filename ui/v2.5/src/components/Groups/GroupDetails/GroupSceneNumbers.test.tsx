import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import type { ComponentProps, ReactNode } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import type { NumberField } from "src/utils/form";
import type * as GQL from "src/core/generated-graphql";
import { GroupSceneNumbers } from "./GroupSceneNumbers";

const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  buttons: [] as { onClick: () => Promise<void> | void; children: ReactNode }[],
  fields: [] as ComponentProps<typeof NumberField>[],
  update: vi.fn(),
  refetch: vi.fn(),
  error: vi.fn(),
  canModify: true,
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useState: (initial: unknown) => {
    const index = page.next++;
    if (!(index in page.state)) page.state[index] = initial;
    return [
      page.state[index],
      (value: unknown) => {
        page.state[index] =
          typeof value === "function" ? value(page.state[index]) : value;
      },
    ];
  },
}));
vi.mock("react-bootstrap", async (original) => ({
  ...(await original<typeof import("react-bootstrap")>()),
  Button: (props: (typeof page.buttons)[number]) => {
    page.buttons.push(props);
    return <button type="button">{props.children}</button>;
  },
}));
vi.mock("src/utils/form", () => ({
  NumberField: (props: ComponentProps<typeof NumberField>) => {
    page.fields.push(props);
    return (
      <input readOnly value={props.value} aria-invalid={props.isInvalid} />
    );
  },
}));
vi.mock("src/core/StashService", () => ({
  useSceneUpdate: () => [page.update],
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({ error: page.error }) }));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanModify: () => page.canModify,
}));

const scenes = [
  {
    id: "1",
    title: "First",
    groups: [
      { group: { id: "10" }, scene_index: 1 },
      { group: { id: "20" }, scene_index: 9 },
    ],
  },
  {
    id: "2",
    title: "Second",
    groups: [{ group: { id: "10" }, scene_index: 2 }],
  },
  {
    id: "3",
    title: "Descendant",
    groups: [{ group: { id: "30" }, scene_index: 3 }],
  },
] as GQL.SlimSceneDataFragment[];

function render(groupId: string | undefined = "10") {
  page.next = 0;
  page.buttons = [];
  page.fields = [];
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" onError={() => {}}>
      <GroupSceneNumbers
        scenes={scenes}
        groupId={groupId}
        onSaved={page.refetch}
      >
        Normal list
      </GroupSceneNumbers>
    </IntlProvider>
  );
}
function edit() {
  render();
  expect(page.buttons).toHaveLength(1);
  page.buttons[0].onClick();
  render();
}
function change(index: number, value: string, badInput = false) {
  page.fields[index].onChange!({
    currentTarget: { value, validity: { badInput } },
  } as Parameters<
    NonNullable<ComponentProps<typeof NumberField>["onChange"]>
  >[0]);
  return render();
}
beforeEach(() => {
  page.state = [];
  page.canModify = true;
  vi.clearAllMocks();
  page.update.mockResolvedValue({});
});

it("saves one update per changed scene and preserves the other groups", async () => {
  edit();
  expect(page.fields).toHaveLength(2);
  change(0, "4");
  change(1, "5");
  await page.buttons[0].onClick();
  expect(page.update.mock.calls).toEqual([
    [
      {
        variables: {
          input: {
            id: "1",
            groups: [
              { group_id: "10", scene_index: 4 },
              { group_id: "20", scene_index: 9 },
            ],
          },
        },
      },
    ],
    [
      {
        variables: {
          input: { id: "2", groups: [{ group_id: "10", scene_index: 5 }] },
        },
      },
    ],
  ]);
});
it("does not send unchanged scenes", async () => {
  edit();
  change(0, "4");
  await page.buttons[0].onClick();
  expect(page.update).toHaveBeenCalledTimes(1);
  expect(page.update.mock.calls[0][0].variables.input.id).toBe("1");
});
it("discards edits on cancel without sending updates", () => {
  edit();
  change(0, "4");
  page.buttons[1].onClick();
  expect(render()).toContain("Normal list");
  edit();
  expect(page.fields[0].value).toBe("1");
  expect(page.update).not.toHaveBeenCalled();
});
it("clears the number with null", async () => {
  edit();
  change(0, "");
  await page.buttons[0].onClick();
  expect(page.update.mock.calls[0][0].variables.input.groups[0]).toEqual({
    group_id: "10",
    scene_index: null,
  });
});
it.each([
  ["1.5", false],
  ["invalid", false],
  ["", true],
])("rejects invalid input %s (badInput %s)", async (value, badInput) => {
  edit();
  change(1, "5");
  change(0, value, badInput);
  await page.buttons[0].onClick();
  expect(render()).toContain('aria-invalid="true"');
  expect(page.update).not.toHaveBeenCalled();
});
it("hides the toggle outside a group or without modify access", () => {
  render();
  expect(page.buttons).toHaveLength(1);
  render("");
  expect(page.buttons).toHaveLength(0);
  page.canModify = false;
  expect(render()).toContain("Normal list");
  expect(page.buttons).toHaveLength(0);
});

// #1575: saving must refresh the sorted list after the mutation completes.
it("saves a single edited number once, then refreshes the list", async () => {
  edit();
  expect(page.fields[0].value).toBe("1");
  change(0, "2"); // A duplicate of scene 2 is allowed.
  await page.buttons[0].onClick();
  expect(page.update).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: {
        id: "1",
        groups: [
          { group_id: "10", scene_index: 2 },
          { group_id: "20", scene_index: 9 },
        ],
      },
    },
  });
  expect(page.refetch).toHaveBeenCalledOnce();
  expect(page.update.mock.invocationCallOrder[0]).toBeLessThan(
    page.refetch.mock.invocationCallOrder[0]
  );
});
it.each([
  "2147483648",
  "-2147483649",
])("rejects numbers outside GraphQL Int: %s", async (value) => {
  edit();
  change(0, value);
  await page.buttons[0].onClick();
  expect(page.update).not.toHaveBeenCalled();
});
