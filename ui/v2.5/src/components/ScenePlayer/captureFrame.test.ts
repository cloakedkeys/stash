import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { captureFrame } from "./captureFrame";

const drawImage = vi.fn();
const toDataURL = vi.fn(() => "data:image/jpeg;base64,frame");
const getContext = vi.fn(() => ({ drawImage }));
const canvas = { width: 0, height: 0, getContext, toDataURL };
const video = {
  readyState: 2,
  videoWidth: 1920,
  videoHeight: 1080,
} as HTMLVideoElement;

beforeEach(() => {
  vi.clearAllMocks();
  getContext.mockReturnValue({ drawImage });
  toDataURL.mockReturnValue("data:image/jpeg;base64,frame");
  vi.stubGlobal("document", { createElement: vi.fn(() => canvas) });
});
afterEach(() => vi.unstubAllGlobals());

describe("capture a performer picture from the player", () => {
  it("captures the decoded frame at its native dimensions as a data URL", () => {
    expect(captureFrame(video)).toBe("data:image/jpeg;base64,frame");
    expect(canvas.width).toBe(1920);
    expect(canvas.height).toBe(1080);
    expect(drawImage).toHaveBeenCalledWith(video, 0, 0, 1920, 1080);
    expect(toDataURL).toHaveBeenCalledWith("image/jpeg");
  });

  it.each([
    null,
    { ...video, readyState: 1 },
    { ...video, videoWidth: 0 },
    { ...video, videoHeight: 0 },
  ])("rejects an unavailable frame (%s)", (unavailable) => {
    expect(() =>
      captureFrame(unavailable as HTMLVideoElement | null)
    ).toThrow();
    expect(drawImage).not.toHaveBeenCalled();
  });

  it("reports canvas and cross-origin capture failures", () => {
    getContext.mockReturnValueOnce(null!);
    expect(() => captureFrame(video)).toThrow("Canvas unavailable");
    toDataURL.mockImplementationOnce(() => {
      throw new Error("Tainted canvas");
    });
    expect(() => captureFrame(video)).toThrow("Tainted canvas");
  });
});
