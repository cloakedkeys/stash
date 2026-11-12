import { readFileSync } from "node:fs";
import { expect, it } from "vitest";

it("wires selected image operations to the shared picker with modify permission", () => {
  const source = readFileSync("src/components/Images/ImageList.tsx", "utf8");
  const entry = source.slice(
    source.indexOf('id: "actions.add_to_gallery"'),
    source.indexOf('id: "actions.select_all"')
  );
  expect(entry).toContain("requiredRole: GQL.RoleEnum.Modify");
  expect(entry).toContain("isDisplayed: () => hasSelection");
  expect(entry).toContain("<AddToGalleryDialog");
  expect(entry).toContain("imageIds={Array.from(selectedIds.values())}");
  expect(entry).toContain("onClose={onCloseEditDelete}");
});

it("wires the single image menu to the same picker with the current image ID", () => {
  const source = readFileSync(
    "src/components/Images/ImageDetails/Image.tsx",
    "utf8"
  );
  expect(source).toContain('id="actions.add_to_gallery"');
  expect(source).toContain("onClick={() => setIsAddToGalleryOpen(true)}");
  expect(source).toMatch(
    /isAddToGalleryOpen && \([\s\S]*?<AddToGalleryDialog\s+imageIds=\{\[image.id\]\}/
  );
  expect(source).toContain("onClose={() => setIsAddToGalleryOpen(false)}");
});
