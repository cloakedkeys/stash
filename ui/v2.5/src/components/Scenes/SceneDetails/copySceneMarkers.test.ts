import { describe, expect, it, vi } from "vitest";
import { copySceneMarkers, type SplitSceneMarker } from "./copySceneMarkers";

function marker(
  title: string,
  endSeconds: number | null = null
): SplitSceneMarker {
  return {
    title,
    seconds: 12.5,
    end_seconds: endSeconds,
    primary_tag: { id: "3", name: "Primary" },
    tags: [{ id: "4", name: "Other" }],
  };
}

describe("copySceneMarkers", () => {
  it("copies every marker to the new scene with the original fields", async () => {
    const markers = [marker("First", 20.75), marker("Second")];
    const original = JSON.stringify(markers);
    const create = vi.fn(async () => true);

    expect(await copySceneMarkers(markers, "new", create, true)).toBe(0);
    expect(create).toHaveBeenCalledTimes(2);
    expect(create).toHaveBeenNthCalledWith(1, {
      scene_id: "new",
      title: "First",
      seconds: 12.5,
      end_seconds: 20.75,
      primary_tag_id: "3",
      tag_ids: ["4"],
    });
    expect(create).toHaveBeenNthCalledWith(2, {
      scene_id: "new",
      title: "Second",
      seconds: 12.5,
      end_seconds: null,
      primary_tag_id: "3",
      tag_ids: ["4"],
    });
    expect(JSON.stringify(markers)).toBe(original);
  });

  it("counts a rejected copy and still copies the remaining markers", async () => {
    const create = vi.fn(async () => true);
    create.mockRejectedValueOnce(new Error("Copy failed"));

    expect(
      await copySceneMarkers(
        [marker("First"), marker("Second")],
        "new",
        create,
        true
      )
    ).toBe(1);
    expect(create).toHaveBeenCalledTimes(2);
    expect(create).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ title: "Second", scene_id: "new" })
    );
  });

  it("counts an unsuccessful response and continues", async () => {
    const create = vi.fn(async () => true);
    create.mockResolvedValueOnce(false);

    expect(
      await copySceneMarkers(
        [marker("First"), marker("Second")],
        "new",
        create,
        true
      )
    ).toBe(1);
    expect(create).toHaveBeenCalledTimes(2);
  });

  it("does nothing when the checkbox is unticked", async () => {
    const create = vi.fn(async () => true);
    expect(
      await copySceneMarkers([marker("First")], "new", create, false)
    ).toBe(0);
    expect(create).not.toHaveBeenCalled();
  });

  it("does nothing when there are no markers", async () => {
    const create = vi.fn(async () => true);
    expect(await copySceneMarkers([], "new", create, true)).toBe(0);
    expect(create).not.toHaveBeenCalled();
  });
});
