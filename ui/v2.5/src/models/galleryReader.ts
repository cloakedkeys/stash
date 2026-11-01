export const READER_BATCH_SIZE = 20;

interface Batch<T> {
  count: number;
  images: T[];
}

interface ReaderState<T> {
  images: T[];
  loading: boolean;
  error: boolean;
  ended: boolean;
}

// Only advance after success. The synchronous loading guard also covers two
// scroll events (or StrictMode effects) arriving before React renders again.
export class ReaderBatches<T extends { id: string }> {
  private page = 1;
  private listeners = new Set<() => void>();
  private state: ReaderState<T> = {
    images: [],
    loading: false,
    error: false,
    ended: false,
  };

  constructor(
    private fetchBatch: (page: number, perPage: number) => Promise<Batch<T>>
  ) {}

  getSnapshot = () => this.state;

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  private update(change: Partial<ReaderState<T>>) {
    this.state = { ...this.state, ...change };
    this.listeners.forEach((listener) => {
      listener();
    });
  }

  loadNext = async () => {
    if (this.state.loading || this.state.ended) return;
    this.update({ loading: true, error: false });
    try {
      const batch = await this.fetchBatch(this.page, READER_BATCH_SIZE);
      const offset = (this.page - 1) * READER_BATCH_SIZE;
      const expected = Math.min(
        READER_BATCH_SIZE,
        Math.max(0, batch.count - offset)
      );
      const seen = new Set(this.state.images.map((image) => image.id));
      // Reject partial or overlapping responses instead of silently skipping
      // pages. Retrying keeps the same server page and the last good images.
      if (
        batch.images.length !== expected ||
        batch.images.some((image) => {
          if (seen.has(image.id)) return true;
          seen.add(image.id);
          return false;
        })
      )
        throw new Error("Incomplete gallery batch");
      this.page += 1;
      this.update({
        images: [...this.state.images, ...batch.images],
        loading: false,
        ended: offset + batch.images.length >= batch.count,
      });
    } catch {
      this.update({ loading: false, error: true });
    }
  };
}

export interface ReaderDimensions {
  width: number;
  height: number;
}

// Keep two viewport heights of decoded images on either side. All other
// pages retain exactly the same layout height, including after a resize.
export function readerLayout(
  pages: ReaderDimensions[],
  width: number,
  scrollTop: number,
  viewportHeight: number
) {
  let top = 0;
  return pages.map((page, index) => {
    const height =
      width *
      (page.width > 0 && page.height > 0 ? page.height / page.width : 1.5);
    const mounted =
      top + height >= scrollTop - viewportHeight * 2 &&
      top <= scrollTop + viewportHeight * 3;
    top += height;
    return { index, height, mounted };
  });
}
