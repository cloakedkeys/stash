import { describe, expect, it } from "vitest";
import { scrapedStoryTextFields, storyTextLength } from "./storyScrape";

describe("storyTextLength", () => {
  it("counts words across lines and paragraphs", () => {
    expect(storyTextLength("  Once upon\na time.\n\nThe end. ")).toEqual({
      words: 6,
      characters: 30,
    });
  });

  it("counts a character outside ASCII as one", () => {
    expect(storyTextLength("café 😀")).toEqual({ words: 2, characters: 6 });
  });

  it("counts nothing in empty text", () => {
    expect(storyTextLength("")).toEqual({ words: 0, characters: 0 });
  });
});

describe("scrapedStoryTextFields", () => {
  it("sets the fields chosen in the dialog", () => {
    expect(
      scrapedStoryTextFields({ title: "A Title", author: "Someone" })
    ).toEqual([
      ["title", "A Title"],
      ["author", "Someone"],
    ]);
  });

  it("clears a field that was chosen and emptied", () => {
    expect(scrapedStoryTextFields({ tagline: "", title: "A Title" })).toEqual([
      ["title", "A Title"],
      ["tagline", ""],
    ]);
  });

  it("leaves alone fields the dialog didn't set", () => {
    expect(scrapedStoryTextFields({ details: null, date: undefined })).toEqual(
      []
    );
  });
});
