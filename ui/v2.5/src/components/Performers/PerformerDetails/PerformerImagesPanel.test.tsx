import type { ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";
import type * as GQL from "src/core/generated-graphql";
import type { FilteredImageList } from "src/components/Images/ImageList";
import type { ListFilterModel } from "src/models/list-filter/filter";
import messages from "src/locales/en-GB.json";
import { PerformerImagesPanel } from "./PerformerImagesPanel";

const page = vi.hoisted(() => ({
  list: undefined as ComponentProps<typeof FilteredImageList> | undefined,
  update: vi.fn(),
  convert: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("src/components/Images/ImageList", () => ({
  FilteredImageList: (props: ComponentProps<typeof FilteredImageList>) => {
    page.list = props;
    return null;
  },
}));
vi.mock("src/core/StashService", () => ({
  usePerformerUpdate: () => [page.update],
}));
vi.mock("src/core/performers", () => ({
  usePerformerFilterHook: () => undefined,
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: page.success, error: page.error }),
}));
vi.mock("src/utils/image", () => ({
  default: { imageToDataURL: page.convert },
}));

it("uses the full still image but refuses clips without reading or updating", async () => {
  // Use the real locale so the error assertion also checks the displayed text.
  renderToStaticMarkup(
    <IntlProvider
      locale="en-GB"
      messages={{
        ...Object.fromEntries(
          Object.entries(messages.toast).map(([key, value]) => [
            `toast.${key}`,
            value,
          ])
        ),
        performer: messages.performer,
        "actions.set_as_performer_image":
          messages.actions.set_as_performer_image,
      }}
    >
      <PerformerImagesPanel
        active
        performer={{ id: "performer" } as GQL.PerformerDataFragment}
      />
    </IntlProvider>
  );
  const operation = page.list!.extraOperations![0];
  const run = (__typename: "ImageFile" | "VideoFile") =>
    operation.onClick(
      {
        data: {
          findImages: {
            images: [
              {
                id: "image",
                paths: { image: "/full-file", thumbnail: "/thumbnail" },
                visual_files: [{ __typename }],
              },
            ],
          },
        },
      } as GQL.FindImagesQueryResult,
      {} as ListFilterModel,
      new Set(["image"])
    );
  page.convert.mockResolvedValue("data:image/jpeg;base64,full");
  await run("ImageFile");
  expect(page.convert).toHaveBeenCalledExactlyOnceWith("/full-file");
  expect(page.update).toHaveBeenCalledExactlyOnceWith({
    variables: {
      input: { id: "performer", image: "data:image/jpeg;base64,full" },
    },
  });
  expect(page.success).toHaveBeenCalledOnce();
  expect(page.error).not.toHaveBeenCalled();

  vi.clearAllMocks();
  await run("VideoFile");
  expect(page.error).toHaveBeenCalledExactlyOnceWith(
    new Error("Video clips can't be used as a picture")
  );
  expect(page.convert).not.toHaveBeenCalled();
  expect(page.update).not.toHaveBeenCalled();
  expect(page.success).not.toHaveBeenCalled();
});
