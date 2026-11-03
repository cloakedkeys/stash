import * as GQL from "src/core/generated-graphql";
import { scrapedStoryTextFields } from "./StoryDetails/storyScrape";

// The story to add for a scraped one. The address it was imported from stays
// with it, first, unless the scraper already gave that address. Only tags
// already in Stash are added; the scraper's other tags are left out.
export function storyImportInput(
  scraped: GQL.ScrapedStoryDataFragment,
  url: string
): GQL.StoryCreateInput {
  const input: GQL.StoryCreateInput = {};
  for (const [field, value] of scrapedStoryTextFields(scraped)) {
    input[field] = value;
  }

  const urls = scraped.urls ?? [];
  input.urls = urls.includes(url) ? urls : [url, ...urls];

  if (scraped.text) input.text = scraped.text;

  const tagIDs = (scraped.tags ?? []).flatMap((t) =>
    t.stored_id ? [t.stored_id] : []
  );
  if (tagIDs.length > 0) input.tag_ids = tagIDs;

  return input;
}

// A story found by its web address, with all its addresses.
export interface IStoryWithURLs {
  id: string;
  title?: string | null;
  urls: string[];
}

export type StoryImportResult =
  | { kind: "no_scraper" }
  | { kind: "not_found" }
  | { kind: "exists"; story: IStoryWithURLs }
  | { kind: "created"; id: string };

// What importing needs from the server, passed in so it can be tested.
export interface IStoryImportSteps {
  scrapable: (url: string) => boolean;
  // stories that may have the address: the server's match ignores case and
  // treats _ and % as wildcards, so only an exact address counts here
  findByURL: (url: string) => Promise<IStoryWithURLs[]>;
  scrape: (
    url: string
  ) => Promise<GQL.ScrapedStoryDataFragment | null | undefined>;
  create: (input: GQL.StoryCreateInput) => Promise<string>;
}

// Adds the story at a web address to Stories (#1964), unless no installed
// scraper handles the address, the scraper finds nothing there, or the story
// is already in Stories under that address or one the scraper gives.
export async function importStory(
  url: string,
  steps: IStoryImportSteps
): Promise<StoryImportResult> {
  if (!steps.scrapable(url)) return { kind: "no_scraper" };

  const findExact = async (address: string) =>
    (await steps.findByURL(address)).find((s) => s.urls.includes(address));

  // look before fetching, so a story already added isn't fetched again
  const existing = await findExact(url);
  if (existing) return { kind: "exists", story: existing };

  const scraped = await steps.scrape(url);
  if (!scraped) return { kind: "not_found" };

  for (const other of new Set(scraped.urls ?? [])) {
    if (other === url) continue;
    const story = await findExact(other);
    if (story) return { kind: "exists", story };
  }

  return {
    kind: "created",
    id: await steps.create(storyImportInput(scraped, url)),
  };
}
