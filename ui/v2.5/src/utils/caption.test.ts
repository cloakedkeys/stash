import { describe, expect, it } from "vitest";
import { CriterionModifier } from "src/core/generated-graphql";
import { CaptionCriterion } from "src/models/list-filter/criteria/captions";
import { currentLanguageName, languageMap, valueToCode } from "./caption";

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

  it("loads old names in saved and linked filters as the current name", () => {
    const saved = new CaptionCriterion();
    saved.setFromSavedCriterion({
      modifier: CriterionModifier.Includes,
      value: "Holandés",
    });
    expect(saved.value).toBe("Nederlands");
    expect(saved.toCriterionInput().value).toBe("nl");

    const linked = new CaptionCriterion();
    linked.fromDecodedParams({
      modifier: CriterionModifier.Excludes,
      value: "日本",
    });
    expect(linked.value).toBe("日本語");
    expect(linked.toCriterionInput().value).toBe("ja");

    expect(currentLanguageName("English")).toBe("English");
    expect(currentLanguageName("Klingon")).toBe("Klingon");
  });

  it("returns nothing for empty or unknown names", () => {
    expect(valueToCode("")).toBeUndefined();
    expect(valueToCode(null)).toBeUndefined();
    expect(valueToCode("Klingon")).toBeUndefined();
  });
});
