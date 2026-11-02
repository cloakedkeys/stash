import { describe, expect, it } from "vitest";
import {
  performerCardImages,
  performerHoverImage,
  performerImages,
} from "./performerCardImage";

const three = {
  image_path: "/a",
  alternate_images: [{ path: "/b" }, { path: "/c" }],
};

describe("performerImages", () => {
  it("lists the main image first, then the others in order", () => {
    expect(performerImages(three)).toEqual(["/a", "/b", "/c"]);
  });

  it("drops empty and repeated paths", () => {
    expect(
      performerImages({
        image_path: "/a",
        alternate_images: [{ path: "" }, { path: "/a" }, { path: null }],
      })
    ).toEqual(["/a"]);
  });
});

describe("performerCardImages", () => {
  it("shows the main image and the next one on hover by default", () => {
    expect(performerCardImages(three)).toEqual({
      image: "/a",
      hoverImage: "/b",
    });
  });

  it("shows any image, with the one after it on hover", () => {
    expect(performerCardImages(three, 1)).toEqual({
      image: "/b",
      hoverImage: "/c",
    });
    expect(performerCardImages(three, 2)).toEqual({
      image: "/c",
      hoverImage: "/a",
    });
  });

  it("wraps round an index past either end", () => {
    expect(performerCardImages(three, 4).image).toBe("/b");
    expect(performerCardImages(three, -1).image).toBe("/c");
  });

  it("has no hover image with one image", () => {
    expect(performerCardImages({ image_path: "/a" }, 3)).toEqual({
      image: "/a",
      hoverImage: undefined,
    });
  });
});

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
