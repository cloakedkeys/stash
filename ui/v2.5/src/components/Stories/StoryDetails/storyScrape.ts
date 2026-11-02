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
