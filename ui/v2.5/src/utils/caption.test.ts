import { describe, expect, it } from "vitest";
import { CriterionModifier } from "src/core/generated-graphql";
import { CaptionCriterion } from "src/models/list-filter/criteria/captions";
import {
  canonicalLanguageCode,
  currentLanguageName,
  languageMap,
  languageName,
  valueToCode,
} from "./caption";

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

  it("returns nothing for empty names", () => {
    expect(valueToCode("")).toBeUndefined();
    expect(valueToCode(null)).toBeUndefined();
  });

  it("sends an unknown name as it is instead of an empty code", () => {
    expect(valueToCode("Klingon")).toBe("Klingon");
    expect(valueToCode("jpn")).toBe("jpn");
  });
});

describe("caption language codes", () => {
  it("names two-letter codes from the list", () => {
    expect(languageName("ja")).toBe("日本語");
    expect(languageName("nl")).toBe("Nederlands");
    expect(languageName("00")).toBe("Unknown");
  });

  it("names three-letter codes", () => {
    expect(languageName("jpn")).toBe("日本語");
    expect(languageName("eng")).toBe("English");
    expect(languageName("ger")).toBe("Deutsch");
    expect(languageName("deu")).toBe("Deutsch");
    expect(languageName("dut")).toBe("Nederlands");
    expect(languageName("fre")).toBe("Français");
  });

  it("names languages missing from the list", () => {
    expect(languageName("zh")).toBe("中文");
    expect(languageName("chi")).toBe("中文");
    expect(languageName("pt-BR")).toBe("Português (Brasil)");
  });

  it("names the undetermined code as unknown", () => {
    expect(languageName("und")).toBe("Unknown");
  });

  it("falls back to the code itself", () => {
    expect(languageName("xyz")).toBe("xyz");
    expect(languageName("not a code")).toBe("not a code");
    expect(languageName("")).toBe("");
  });

  it("turns three-letter codes into the short form", () => {
    expect(canonicalLanguageCode("jpn")).toBe("ja");
    expect(canonicalLanguageCode("ger")).toBe("de");
    expect(canonicalLanguageCode("en")).toBe("en");
    expect(canonicalLanguageCode("not a code")).toBe("not a code");
  });
});
