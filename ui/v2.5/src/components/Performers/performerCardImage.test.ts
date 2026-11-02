import { describe, expect, it } from "vitest";
import { performerHoverImage } from "./performerCardImage";

describe("performerHoverImage", () => {
  it("uses the first other image", () => {
    expect(
      performerHoverImage({
        image_path: "/performer/1/image?t=1",
        alternate_images: [
          { path: "/performer/1/image/b?t=b" },
          { path: "/performer/1/image/c?t=c" },
        ],
      })
    ).toBe("/performer/1/image/b?t=b");
  });

  it("gives nothing without other images", () => {
    expect(performerHoverImage({ image_path: "/a" })).toBeUndefined();
    expect(
      performerHoverImage({ image_path: "/a", alternate_images: [] })
    ).toBeUndefined();
    expect(
      performerHoverImage({ image_path: "/a", alternate_images: null })
    ).toBeUndefined();
  });

  it("skips empty paths and the main image", () => {
    expect(
      performerHoverImage({
        image_path: "/a",
        alternate_images: [{ path: "" }, { path: "/a" }, { path: "/b" }],
      })
    ).toBe("/b");
  });
});
