import { describe, expect, it } from "vitest";
import { createMouseGesture } from "./mouseGesture";

function point(pageX: number, pageY: number, timeStamp = 1000) {
  return { pageX, pageY, timeStamp };
}

describe("lightbox mouse gestures", () => {
  it("rejects a fast drag ending at the last move (#1622)", () => {
    const gesture = createMouseGesture(point(100, 100));
    gesture.move(point(140, 100, 1050));
    expect(gesture.isClick(point(140, 100, 1080))).toBe(false);
  });

  it("remembers a drag even when it returns to the press", () => {
    const gesture = createMouseGesture(point(100, 100));
    gesture.move(point(140, 100, 1050));
    gesture.move(point(100, 100, 1080));
    expect(gesture.isClick(point(100, 100, 1100))).toBe(false);
    expect(gesture.dragged).toBe(true);
  });

  it("checks release movement even without a mousemove (#1623)", () => {
    const gesture = createMouseGesture(point(100, 100));
    expect(gesture.isClick(point(110, 100, 1050))).toBe(false);
    expect(gesture.dragged).toBe(true);
  });

  it("allows a stationary click and a few pixels of hand movement", () => {
    const click = createMouseGesture(point(100, 100));
    expect(click.isClick(point(100, 100, 1100))).toBe(true);
    expect(click.isClick(point(103, 104, 1100))).toBe(true);
  });

  it("measures the threshold from the press, not each individual move", () => {
    const gesture = createMouseGesture(point(100, 100));
    gesture.move(point(102, 100, 1010));
    gesture.move(point(104, 100, 1020));
    gesture.move(point(106, 100, 1030));
    expect(gesture.isClick(point(106, 100, 1040))).toBe(false);
  });

  it("detects movement in both axes and directions", () => {
    for (const [x, y] of [
      [106, 100],
      [94, 100],
      [100, 106],
      [100, 94],
      [104, 104],
    ]) {
      const gesture = createMouseGesture(point(100, 100));
      expect(gesture.isClick(point(x, y, 1050))).toBe(false);
    }
  });

  it("preserves the existing 200 ms click duration limit", () => {
    const click = createMouseGesture(point(100, 100));
    expect(click.isClick(point(100, 100, 1200))).toBe(true);
    expect(click.isClick(point(100, 100, 1201))).toBe(false);
  });

  it("keeps incremental pan deltas separate from the click decision", () => {
    const gesture = createMouseGesture(point(100, 100));
    expect(gesture.move(point(110, 120))).toEqual([10, 20]);
    expect(gesture.move(point(115, 118))).toEqual([5, -2]);
    expect(gesture.isClick(point(115, 118, 1100))).toBe(false);
  });
});
