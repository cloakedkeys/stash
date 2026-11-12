import React from "react";
import { readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
// The old UI has no set-as component. Render its empty contribution so each
// regression can run against the original menu, rather than fail to import.
const modules = import.meta.glob<{
  ImageSetAsMenu: React.FC<{ image: GQL.ImageDataFragment }>;
}>("./ImageSetAsMenu.tsx", { eager: true });
const ImageSetAsMenu =
  modules["./ImageSetAsMenu.tsx"]?.ImageSetAsMenu ?? (() => null);

interface Target {
  id: string;
  name: string;
}
interface Control {
  children?: React.ReactNode;
  onClick?: () => Promise<void> | void;
  disabled?: boolean;
}
interface Picker {
  creatable: boolean;
  isDisabled: boolean;
  onSelect: (targets: Target[]) => void;
}
const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  items: [] as Control[],
  buttons: [] as Control[],
  picker: undefined as Picker | undefined,
  close: undefined as (() => void) | undefined,
  performer: vi.fn(),
  tag: vi.fn(),
  convert: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  useState: (initial: unknown) => {
    const index = page.next++;
    if (!(index in page.state)) page.state[index] = initial;
    return [
      page.state[index],
      (value: unknown) => {
        page.state[index] = value;
      },
    ];
  },
  useRef: () => {
    const index = page.next++;
    if (!(index in page.state)) page.state[index] = { current: false };
    return page.state[index];
  },
}));
vi.mock("react-bootstrap", () => ({
  Dropdown: {
    Item: (props: Control) => {
      page.items.push(props);
      return <>{props.children}</>;
    },
  },
  Button: (props: Control) => {
    page.buttons.push(props);
    return <button disabled={props.disabled}>{props.children}</button>;
  },
}));
vi.mock("src/components/Shared/Modal", () => ({
  ModalComponent: (props: {
    children: React.ReactNode;
    onHide: () => void;
  }) => {
    page.close = props.onHide;
    return <>{props.children}</>;
  },
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
  FormattedMessage: ({ id }: { id: string }) => <>{id}</>,
}));
vi.mock("react-router-dom", () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}));
vi.mock("src/core/StashService", () => ({
  usePerformerUpdate: () => [page.performer],
  useTagUpdate: () => [page.tag],
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: page.success, error: page.error }),
}));
vi.mock("src/utils/image", () => ({
  default: { imageToDataURL: page.convert },
}));
vi.mock("src/components/Performers/PerformerSelect", () => ({
  PerformerSelect: (props: Picker) => {
    page.picker = props;
    return <input aria-label="performer search" />;
  },
}));
vi.mock("src/components/Tags/TagSelect", () => ({
  TagSelect: (props: Picker) => {
    page.picker = props;
    return <input aria-label="tag search" />;
  },
}));

const image = {
  id: "image",
  paths: { image: "/full-file", thumbnail: "/thumbnail" },
  visual_files: [{ __typename: "ImageFile" }],
  performers: [
    { id: "p1", name: "Linked performer" },
    { id: "p2", name: "Other performer" },
  ],
  tags: [
    { id: "t1", name: "Linked tag" },
    { id: "t2", name: "Other tag" },
  ],
  organized: true,
  rating100: 60,
} as unknown as GQL.ImageDataFragment;
function render(selectedImage = image) {
  page.next = 0;
  page.items = [];
  page.buttons = [];
  page.picker = undefined;
  page.close = undefined;
  return renderToStaticMarkup(<ImageSetAsMenu image={selectedImage} />);
}
beforeEach(() => {
  vi.clearAllMocks();
  page.state = [];
  page.convert.mockResolvedValue("data:image/jpeg;base64,full");
  page.performer.mockResolvedValue({});
  page.tag.mockResolvedValue({});
});

it("is mounted in the image operations menu", () => {
  const source = readFileSync(new URL("./Image.tsx", import.meta.url), "utf8");
  expect(source).toMatch(
    /<Dropdown.Menu[^>]*>\s*<ImageSetAsMenu image=\{image\} \/>/
  );
});

it("hides both picture actions for a video clip", () => {
  const markup = render({
    ...image,
    visual_files: [{ __typename: "VideoFile" }],
  } as GQL.ImageDataFragment);
  expect(page.items).toHaveLength(0);
  expect(markup).not.toContain("actions.set_as_performer_image");
  expect(markup).not.toContain("actions.set_as_tag_image");
  expect(page.convert).not.toHaveBeenCalled();
  expect(page.performer).not.toHaveBeenCalled();
  expect(page.tag).not.toHaveBeenCalled();
});

for (const [index, type, targets] of [
  [0, "performer", image.performers],
  [1, "tag", image.tags],
] as const) {
  describe(`${type} picture menu`, () => {
    function open() {
      render();
      expect(
        page.items[index],
        `${type} picture action is available`
      ).toBeDefined();
      expect(
        renderToStaticMarkup(<span>{page.items[index].children}</span>)
      ).toContain(`actions.set_as_${type}_image`);
      page.items[index].onClick!();
      return render();
    }
    it("lists linked targets before the existing search picker", () => {
      const markup = open();
      expect(
        page.buttons.map((button) =>
          renderToStaticMarkup(<span>{button.children}</span>)
        )
      ).toEqual(targets.map((target) => `<span>${target.name}</span>`));
      expect(markup.indexOf(targets[0].name)).toBeLessThan(
        markup.indexOf(`${type} search`)
      );
      expect(page.picker?.creatable).toBe(false);
      expect(page.performer).not.toHaveBeenCalled();
      expect(page.tag).not.toHaveBeenCalled();
    });
    it("sets only the chosen linked target's picture from the full file, with a linked toast", async () => {
      const before = JSON.stringify(image);
      open();
      await page.buttons[1].onClick!();
      expect(page.convert).toHaveBeenCalledExactlyOnceWith("/full-file");
      expect(
        type === "performer" ? page.performer : page.tag
      ).toHaveBeenCalledExactlyOnceWith({
        variables: {
          input: { id: targets[1].id, image: "data:image/jpeg;base64,full" },
        },
      });
      expect(
        type === "performer" ? page.tag : page.performer
      ).not.toHaveBeenCalled();
      expect(JSON.stringify(image)).toBe(before);
      expect(renderToStaticMarkup(page.success.mock.calls[0][0])).toContain(
        `href="/${type === "performer" ? "performers" : "tags"}/${targets[1].id}"`
      );
      render();
      expect(page.picker).toBeUndefined();
    });
    it("allows a target from the existing search without changing image associations", async () => {
      const before = JSON.stringify(image);
      open();
      page.picker!.onSelect([{ id: "searched", name: "Search result" }]);
      await vi.waitFor(() => expect(page.success).toHaveBeenCalledOnce());
      expect(
        type === "performer" ? page.performer : page.tag
      ).toHaveBeenCalledExactlyOnceWith({
        variables: {
          input: { id: "searched", image: "data:image/jpeg;base64,full" },
        },
      });
      expect(
        type === "performer" ? page.tag : page.performer
      ).not.toHaveBeenCalled();
      expect(JSON.stringify(image)).toBe(before);
    });
    it("cancels without reading or updating anything", () => {
      open();
      page.close!();
      render();
      expect(page.picker).toBeUndefined();
      expect(page.convert).not.toHaveBeenCalled();
      expect(page.performer).not.toHaveBeenCalled();
      expect(page.tag).not.toHaveBeenCalled();
    });
    it("reports failures and keeps the picker available", async () => {
      page.convert.mockRejectedValueOnce(new Error("Read failed"));
      open();
      await page.buttons[0].onClick!();
      render();
      expect(page.error).toHaveBeenCalledOnce();
      expect(page.success).not.toHaveBeenCalled();
      expect(page.performer).not.toHaveBeenCalled();
      expect(page.tag).not.toHaveBeenCalled();
      expect(page.picker?.isDisabled).toBe(false);
    });
    it("blocks duplicate saves and closing while saving", async () => {
      let finish!: (data: string) => void;
      page.convert.mockImplementationOnce(
        () =>
          new Promise<string>((resolve) => {
            finish = resolve;
          })
      );
      open();
      const saving = page.buttons[0].onClick!();
      await page.buttons[1].onClick!();
      page.close!();
      render();
      expect(page.picker?.isDisabled).toBe(true);
      expect(page.buttons.every((button) => button.disabled)).toBe(true);
      finish("data:image/jpeg;base64,full");
      await saving;
      expect(page.convert).toHaveBeenCalledOnce();
      expect(
        type === "performer" ? page.performer : page.tag
      ).toHaveBeenCalledOnce();
    });
  });
}
