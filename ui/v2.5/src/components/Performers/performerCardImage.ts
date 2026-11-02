export interface IPerformerAlternateImages {
  image_path?: string | null;
  alternate_images?: readonly { path?: string | null }[] | null;
}

// All of a performer's images, the main one first, without empty or repeated
// paths.
export function performerImages(
  performer: IPerformerAlternateImages
): string[] {
  const paths = [
    performer.image_path,
    ...(performer.alternate_images ?? []).map((i) => i.path),
  ];
  return [...new Set(paths.filter((p): p is string => !!p))];
}

// What a performer card shows: the image at `index` (wrapping round), and on
// hover the one after it. `index` 0 is the main image.
export function performerCardImages(
  performer: IPerformerAlternateImages,
  index = 0
): { image?: string; hoverImage?: string } {
  const images = performerImages(performer);
  if (images.length === 0) return { image: performer.image_path ?? undefined };
  const i = ((index % images.length) + images.length) % images.length;
  return {
    image: images[i],
    hoverImage: images.length > 1 ? images[(i + 1) % images.length] : undefined,
  };
}

// The image a performer card shows on hover, when it shows the main image:
// the first other image.
export function performerHoverImage(
  performer: IPerformerAlternateImages
): string | undefined {
  return performerCardImages(performer).hoverImage;
}
