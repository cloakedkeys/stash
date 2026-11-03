import { afterEach, describe, expect, it, vi } from "vitest";
import { coverToDataURL } from "./SceneCoverPicker";

// what FileReader gives for a blob, without a browser
class FakeReader {
  result: string | null = null;
  onload: (() => void) | null = null;
  onloadend: (() => void) | null = null;
  readAsDataURL(blob: Blob) {
    this.result = `data:${blob.type};base64,AAAA`;
    this.onload?.();
    this.onloadend?.();
  }
}

describe("coverToDataURL (#1977)", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reads a scene's cover as image data", async () => {
    vi.stubGlobal("FileReader", FakeReader);
    const fetchCover = vi.fn(
      async () =>
        new Response(new Blob(["jpeg"], { type: "image/jpeg" }), {
          status: 200,
        })
    );
    vi.stubGlobal("fetch", fetchCover);

    expect(await coverToDataURL("/scene/3/screenshot?t=1")).toBe(
      "data:image/jpeg;base64,AAAA"
    );
    expect(fetchCover).toHaveBeenCalledWith("/scene/3/screenshot?t=1");
  });

  it("fails on a cover that doesn't load, rather than setting an error page as the picture", async () => {
    vi.stubGlobal("FileReader", FakeReader);
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("not found", { status: 404, statusText: "Not Found" })
      )
    );

    await expect(coverToDataURL("/scene/3/screenshot")).rejects.toThrow(
      "Not Found"
    );
  });
});
