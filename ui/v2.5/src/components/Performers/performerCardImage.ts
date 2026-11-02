export interface IPerformerAlternateImages {
  image_path?: string | null;
  alternate_images?: readonly { path?: string | null }[] | null;
}

// The image a performer card shows on hover: the first other image that
// isn't the main one.
export function performerHoverImage(
  performer: IPerformerAlternateImages
): string | undefined {
  return (
    performer.alternate_images?.find(
      (i) => !!i.path && i.path !== performer.image_path
    )?.path ?? undefined
  );
}
