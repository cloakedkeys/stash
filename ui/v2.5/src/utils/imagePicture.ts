import type { IntlShape } from "react-intl";
import type { ImageDataFragment } from "src/core/generated-graphql";
import ImageUtils from "./image";
import { isVideo } from "./visualFile";

type PictureImage = Pick<ImageDataFragment, "paths" | "visual_files">;

export function isImageClip(image: PictureImage) {
  const file = image.visual_files[0];
  return !!file && isVideo(file);
}

export async function imageToPictureDataURL(
  image: PictureImage | undefined,
  intl: IntlShape
) {
  if (image && isImageClip(image)) {
    throw new Error(intl.formatMessage({ id: "toast.video_clip_picture" }));
  }
  const imagePath = image?.paths.image;
  if (!imagePath) {
    throw new Error("Selected image does not have an image path");
  }
  return ImageUtils.imageToDataURL(imagePath);
}
