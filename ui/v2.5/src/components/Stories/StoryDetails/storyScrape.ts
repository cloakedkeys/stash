import * as GQL from "src/core/generated-graphql";

const textFields = [
  "title",
  "author",
  "source_website",
  "date",
  "language",
  "tagline",
  "details",
] as const;

type StoryTextField = (typeof textFields)[number];

// The text fields to change after the scrape dialog's Apply. A field the
// dialog left alone isn't set; one chosen and cleared becomes empty.
export function scrapedStoryTextFields(
  data: GQL.ScrapedStoryDataFragment
): [StoryTextField, string][] {
  const ret: [StoryTextField, string][] = [];
  for (const field of textFields) {
    const value = data[field];
    if (value !== undefined && value !== null) ret.push([field, value]);
  }
  return ret;
}

// How long a story's text is, to show in place of the text itself, which is
// too long to show in the scrape dialog.
export function storyTextLength(text: string): {
  words: number;
  characters: number;
} {
  const words = text.split(/\s+/).filter((w) => w !== "").length;
  // counted by character, not by UTF-16 unit, so "é" or an emoji is one
  const characters = Array.from(text).length;
  return { words, characters };
}
