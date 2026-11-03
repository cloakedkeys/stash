import { describe, expect, it } from "vitest";
import { canExpandToast } from "./Toast";

describe("canExpandToast", () => {
  it("opens only errors in the box titled Error", () => {
    expect(canExpandToast({ variant: "danger" })).toBe(true);
    expect(canExpandToast({})).toBe(false);
    expect(canExpandToast({ variant: "success" })).toBe(false);
    expect(canExpandToast({ variant: "warning" })).toBe(false);
  });
});
