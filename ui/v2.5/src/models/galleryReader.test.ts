import { describe, expect, it } from "vitest";
import {
  ReaderBatches,
  readerLayout,
  READER_BATCH_SIZE,
} from "./galleryReader";

describe("gallery reader batches", () => {
  it("loads bounded consecutive batches exactly once, including the last partial batch", async () => {
    const pages = Array.from({ length: 43 }, (_, id) => ({ id: String(id) }));
    const requests: number[] = [];
    const reader = new ReaderBatches(async (page, perPage) => {
      requests.push(page);
      expect(perPage).toBe(READER_BATCH_SIZE);
      return {
        count: pages.length,
        images: pages.slice((page - 1) * perPage, page * perPage),
      };
    });
    await Promise.all([reader.loadNext(), reader.loadNext()]);
    expect(reader.getSnapshot().images).toHaveLength(20);
    await reader.loadNext();
    await reader.loadNext();
    await reader.loadNext();
    expect(requests).toEqual([1, 2, 3]);
    expect(reader.getSnapshot().images).toEqual(pages);
    expect(reader.getSnapshot().ended).toBe(true);
  });

  it("retries the failed batch without losing or repeating pages", async () => {
    const requests: number[] = [];
    let fail = true;
    const reader = new ReaderBatches(async (page) => {
      requests.push(page);
      if (page === 2 && fail) throw new Error("offline");
      return {
        count: 21,
        images: Array.from({ length: page === 1 ? 20 : 1 }, (_, i) => ({
          id: String((page - 1) * 20 + i),
        })),
      };
    });
    await reader.loadNext();
    await reader.loadNext();
    expect(reader.getSnapshot()).toMatchObject({
      loading: false,
      error: true,
      ended: false,
    });
    expect(reader.getSnapshot().images).toHaveLength(20);
    fail = false;
    await reader.loadNext();
    expect(requests).toEqual([1, 2, 2]);
    expect(reader.getSnapshot()).toMatchObject({ error: false, ended: true });
    expect(reader.getSnapshot().images.map((image) => image.id)).toEqual(
      Array.from({ length: 21 }, (_, i) => String(i))
    );
  });

  it("rejects duplicate pages without advancing, then accepts a corrected retry", async () => {
    let duplicate = true;
    const requests: number[] = [];
    const reader = new ReaderBatches(async (page) => {
      requests.push(page);
      return {
        count: 21,
        images:
          page === 1
            ? Array.from({ length: 20 }, (_, id) => ({ id: String(id) }))
            : [{ id: duplicate ? "19" : "20" }],
      };
    });
    await reader.loadNext();
    await reader.loadNext();
    expect(reader.getSnapshot()).toMatchObject({ error: true, ended: false });
    expect(reader.getSnapshot().images).toHaveLength(20);
    duplicate = false;
    await reader.loadNext();
    expect(requests).toEqual([1, 2, 2]);
    expect(reader.getSnapshot().images.map((image) => image.id)).toEqual(
      Array.from({ length: 21 }, (_, id) => String(id))
    );
  });

  it("handles empty galleries and does not accept a prematurely empty batch", async () => {
    const empty = new ReaderBatches(async () => ({ count: 0, images: [] }));
    await empty.loadNext();
    expect(empty.getSnapshot().ended).toBe(true);
    const incomplete = new ReaderBatches(async () => ({
      count: 5,
      images: [],
    }));
    await incomplete.loadNext();
    expect(incomplete.getSnapshot()).toMatchObject({
      error: true,
      ended: false,
    });
  });
});

describe("gallery reader image window", () => {
  const pages = Array.from({ length: 30 }, () => ({
    width: 500,
    height: 1000,
  }));

  it("mounts only nearby pages and preserves every placeholder height", () => {
    const layout = readerLayout(pages, 500, 10000, 1000);
    expect(
      layout.filter((page) => page.mounted).map((page) => page.index)
    ).toEqual([7, 8, 9, 10, 11, 12, 13]);
    expect(layout.every((page) => page.height === 1000)).toBe(true);
    expect(layout.reduce((sum, page) => sum + page.height, 0)).toBe(30000);
  });

  it("remounts earlier pages on reverse scrolling, and recalculates on resize", () => {
    expect(
      readerLayout(pages, 500, 0, 1000)
        .filter((page) => page.mounted)
        .map((page) => page.index)
    ).toEqual([0, 1, 2, 3]);
    const resized = readerLayout(pages, 250, 0, 1000);
    expect(resized[0].height).toBe(500);
    expect(resized[6].mounted).toBe(true);
    expect(resized[7].mounted).toBe(false);
  });

  it("uses a stable fallback until missing dimensions are known", () => {
    expect(readerLayout([{ width: 0, height: 0 }], 300, 0, 600)[0].height).toBe(
      450
    );
  });
});
