import React, { type ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { ModalComponent } from "../Shared/Modal";
import { MultiSetModeButtons } from "../Shared/MultiSet";
import { URLListInput } from "../Shared/URLField";
import { EditImagesDialog } from "./EditImagesDialog";

const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  update: vi.fn(),
}));
vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useMemo: (calculate: () => unknown) => calculate(),
  useEffect: () => {},
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
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("src/core/StashService", () => ({
  useBulkImageUpdate: () => [page.update],
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));
vi.mock("src/hooks/CurrentUser", () => ({ useIsAdmin: () => true }));
vi.mock("../Shared/Select", () => ({ StudioSelect: () => null }));
vi.mock("../Shared/Modal", () => ({ ModalComponent: () => null }));
vi.mock("../Shared/MultiSet", () => ({
  MultiSet: () => null,
  MultiSetModeButtons: () => null,
}));
vi.mock("../Shared/URLField", () => ({ URLListInput: () => null }));
vi.mock("../Shared/Rating/RatingSystem", () => ({ RatingSystem: () => null }));
vi.mock("../Shared/IndeterminateCheckbox", () => ({
  IndeterminateCheckbox: () => null,
}));
vi.mock("../Shared/BulkUpdate", () => ({
  BulkUpdateFormGroup: () => null,
  BulkUpdateTextInput: () => null,
}));
vi.mock("../Shared/DateInput", () => ({ BulkUpdateDateInput: () => null }));
vi.mock("src/utils/yup", () => ({ getDateError: () => undefined }));

const selected = ["1", "2"].map((id) => ({
  id,
  urls: ["https://example.com/existing"],
  tags: [],
  performers: [],
  galleries: [],
})) as unknown as GQL.SlimImageDataFragment[];

function elements(
  node: React.ReactNode
): React.ReactElement<{ children?: React.ReactNode; name?: string }>[] {
  return React.Children.toArray(node).flatMap((child) =>
    React.isValidElement<{ children?: React.ReactNode; name?: string }>(child)
      ? [child, ...elements(child.props.children)]
      : []
  );
}

function render() {
  page.next = 0;
  const tree = elements(EditImagesDialog({ selected, onClose: vi.fn() }));
  const group = tree.find((e) => e.props.name === "urls");
  expect(group, "URLs field is available").toBeDefined();
  const controls = elements(group!.props.children);
  return {
    urls: controls.find((e) => e.type === URLListInput)!
      .props as ComponentProps<typeof URLListInput>,
    modes: controls.find((e) => e.type === MultiSetModeButtons)!
      .props as ComponentProps<typeof MultiSetModeButtons>,
    modal: tree.find((e) => e.type === ModalComponent)!.props as ComponentProps<
      typeof ModalComponent
    >,
  };
}

beforeEach(() => {
  page.state = [];
  page.next = 0;
  vi.clearAllMocks();
  page.update.mockResolvedValue({});
  vi.stubGlobal("document", { body: {} });
});
afterEach(() => vi.unstubAllGlobals());

describe("bulk image URLs", () => {
  it.each([
    GQL.BulkUpdateIdMode.Add,
    GQL.BulkUpdateIdMode.Remove,
    GQL.BulkUpdateIdMode.Set,
  ])("sends URLs in %s mode", async (mode) => {
    expect(render().modes.mode).toBe(GQL.BulkUpdateIdMode.Add);
    render().modes.onSetMode(mode);
    const values = ["https://example.com/one", "https://example.com/two"];
    render().urls.setValue(values);
    expect(render().urls.value).toEqual(values);
    expect(render().modes.mode).toBe(mode);
    await render().modal.accept!.onClick!();
    expect(page.update).toHaveBeenCalledWith({
      variables: {
        input: expect.objectContaining({
          ids: ["1", "2"],
          urls: { mode, values },
        }),
      },
    });
  });

  it("omits untouched URLs when applying the dialog", async () => {
    expect(render().urls.value).toEqual([]);
    await render().modal.accept!.onClick!();
    expect(page.update).toHaveBeenCalledOnce();
    expect(page.update.mock.calls[0][0].variables.input).not.toHaveProperty(
      "urls"
    );
  });

  it("can overwrite URLs with an empty list", async () => {
    render().modes.onSetMode(GQL.BulkUpdateIdMode.Set);
    await render().modal.accept!.onClick!();
    expect(page.update.mock.calls[0][0].variables.input.urls).toEqual({
      mode: GQL.BulkUpdateIdMode.Set,
      values: [],
    });
  });
});
