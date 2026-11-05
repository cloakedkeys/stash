import { describe, expect, it } from "vitest";
import { languageMap, valueToCode } from "./caption";

describe("caption language names", () => {
  it("names each language in that language", () => {
    expect(languageMap.get("de")).toBe("Deutsch");
    expect(languageMap.get("ja")).toBe("日本語");
    expect(languageMap.get("ko")).toBe("한국어");
    expect(languageMap.get("nl")).toBe("Nederlands");
  });

  it("finds the code for every shown name", () => {
    for (const [code, name] of languageMap) {
      expect(valueToCode(name)).toBe(code);
    }
  });

  it("still reads filters saved with the old names", () => {
    expect(valueToCode("Deutsche")).toBe("de");
    expect(valueToCode("日本")).toBe("ja");
    expect(valueToCode("한국인")).toBe("ko");
    expect(valueToCode("Holandés")).toBe("nl");
  });

  it("returns nothing for empty or unknown names", () => {
    expect(valueToCode("")).toBeUndefined();
    expect(valueToCode(null)).toBeUndefined();
    expect(valueToCode("Klingon")).toBeUndefined();
  });
});
