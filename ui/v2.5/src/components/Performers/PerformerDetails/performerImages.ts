// Reorder the entire alternate list, including images on other thumbnail pages.
export function movePerformerImage(
  checksums: readonly string[],
  index: number,
  direction: -1 | 1
): string[] {
  const result = [...checksums];
  const target = index + direction;
  if (
    index < 0 ||
    index >= result.length ||
    target < 0 ||
    target >= result.length
  )
    return result;
  [result[index], result[target]] = [result[target], result[index]];
  return result;
}

// The scraped images that weren't picked as the main image, in their scraped
// order and without repeats. Saving adds them to the performer's other images.
export function unpickedImages(
  images: readonly string[],
  picked: string | undefined
): string[] {
  return [...new Set(images)].filter((image) => image && image !== picked);
}

export async function addPerformerImageFiles(
  files: readonly File[],
  add: (image: string) => Promise<unknown>,
  onError: (file: File, error: unknown) => void
) {
  for (const file of files) {
    try {
      const image = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result as string);
        reader.onerror = () => reject(reader.error);
        reader.onabort = () => reject(new Error("Image read aborted"));
        reader.readAsDataURL(file);
      });
      await add(image);
    } catch (error) {
      onError(file, error);
    }
  }
}
