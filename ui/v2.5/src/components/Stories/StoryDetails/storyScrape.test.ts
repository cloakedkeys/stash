import { describe, expect, it } from "vitest";
import { scrapedStoryTextFields } from "./storyScrape";

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
