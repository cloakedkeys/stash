import { describe, expect, it } from "vitest";
import ImageUtils from "./image";

describe("scene cover thumbnail URLs", () => {
  it("keeps the timestamp, API key, base path and fragment", () => {
    const url = ImageUtils.getThumbnailUrl(
      "https://stash.test/base/scene/1/screenshot?t=123&apikey=secret#cover"
    );
    expect(url).toBe(
      "https://stash.test/base/scene/1/screenshot?t=123&apikey=secret&thumbnail=true#cover"
    );
  });

  it("handles a URL without a query and an absent cover", () => {
    expect(ImageUtils.getThumbnailUrl("/scene/1/screenshot")).toBe(
      "/scene/1/screenshot?thumbnail=true"
    );
    expect(ImageUtils.getThumbnailUrl(undefined)).toBeUndefined();
    expect(ImageUtils.getThumbnailUrl(null)).toBeUndefined();
    expect(ImageUtils.getThumbnailUrl("")).toBeUndefined();
  });
});
