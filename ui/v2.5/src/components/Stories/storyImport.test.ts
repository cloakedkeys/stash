import { describe, expect, it, vi } from "vitest";
import * as GQL from "src/core/generated-graphql";
import {
  IStoryImportSteps,
  IStoryWithURLs,
  importStory,
  storyImportInput,
} from "./storyImport";

const url = "https://www.literotica.com/s/a-story";

function scraped(
  data: Partial<GQL.ScrapedStoryDataFragment> = {}
): GQL.ScrapedStoryDataFragment {
  return { __typename: "ScrapedStory", title: "A Story", ...data };
}

function story(id: string, urls = [url]): IStoryWithURLs {
  return { id, title: `Story ${id}`, urls };
}

function steps(over: Partial<IStoryImportSteps> = {}): IStoryImportSteps {
  return {
    scrapable: vi.fn(() => true),
    findByURL: vi.fn(async () => []),
    scrape: vi.fn(async () => scraped({ urls: [url], text: "Once." })),
    create: vi.fn(async () => "7"),
    ...over,
  };
}

describe("importStory (#1964)", () => {
  it("adds the scraped story and returns its id", async () => {
    const s = steps();
    expect(await importStory(url, s)).toEqual({ kind: "created", id: "7" });
    expect(s.findByURL).toHaveBeenCalledWith(url);
    expect(s.create).toHaveBeenCalledWith({
      title: "A Story",
      urls: [url],
      text: "Once.",
    });
  });

  it("says when no installed scraper handles the address, without fetching", async () => {
    const s = steps({ scrapable: vi.fn(() => false) });
    expect(await importStory(url, s)).toEqual({ kind: "no_scraper" });
    expect(s.findByURL).not.toHaveBeenCalled();
    expect(s.scrape).not.toHaveBeenCalled();
    expect(s.create).not.toHaveBeenCalled();
  });

  it("says when the story is already in Stories, without fetching it", async () => {
    const s = steps({ findByURL: vi.fn(async () => [story("3")]) });
    expect(await importStory(url, s)).toEqual({
      kind: "exists",
      story: story("3"),
    });
    expect(s.scrape).not.toHaveBeenCalled();
    expect(s.create).not.toHaveBeenCalled();
  });

  it("finds the story under another address the scraper gives", async () => {
    const canonical = "https://www.literotica.com/s/a-story-1";
    const s = steps({
      scrape: vi.fn(async () => scraped({ urls: [canonical, canonical] })),
      findByURL: vi.fn(async (u: string) =>
        u === canonical ? [story("4", [canonical])] : []
      ),
    });
    expect(await importStory(`${url}?page=2`, s)).toEqual({
      kind: "exists",
      story: story("4", [canonical]),
    });
    expect(s.findByURL).toHaveBeenCalledTimes(2);
    expect(s.create).not.toHaveBeenCalled();
  });

  it("only counts a story with exactly that address", async () => {
    // the server matches without case and with _ and % as wildcards, so it
    // can return part-one for part_one
    const s = steps({
      findByURL: vi.fn(async () => [
        story("5", ["https://www.literotica.com/s/a-Story"]),
        story("6", ["https://www.literotica.com/s/a_story"]),
      ]),
    });
    expect(
      await importStory("https://www.literotica.com/s/a-story", s)
    ).toEqual({ kind: "created", id: "7" });
    expect(s.scrape).toHaveBeenCalledOnce();
  });

  it("says when the scraper finds nothing", async () => {
    const s = steps({ scrape: vi.fn(async () => null) });
    expect(await importStory(url, s)).toEqual({ kind: "not_found" });
    expect(s.create).not.toHaveBeenCalled();
  });

  it("passes on a failed fetch", async () => {
    const s = steps({
      scrape: vi.fn(async () => {
        throw new Error("timed out");
      }),
    });
    await expect(importStory(url, s)).rejects.toThrow("timed out");
    expect(s.create).not.toHaveBeenCalled();
  });
});

describe("storyImportInput", () => {
  it("keeps the pasted address first when the scraper didn't give it", () => {
    expect(
      storyImportInput(scraped({ urls: ["https://example.com/a"] }), url).urls
    ).toEqual([url, "https://example.com/a"]);
    expect(storyImportInput(scraped({ urls: null }), url).urls).toEqual([url]);
  });

  it("keeps the scraper's addresses as they are when it gave the pasted one", () => {
    const urls = ["https://example.com/a", url];
    expect(storyImportInput(scraped({ urls }), url).urls).toEqual(urls);
  });

  it("copies the details and text, and only tags already in Stash", () => {
    expect(
      storyImportInput(
        scraped({
          author: "Someone",
          date: "2020-01-02",
          details: "About it",
          text: "Once upon a time.",
          tags: [
            { __typename: "ScrapedTag", stored_id: "12", name: "Known" },
            { __typename: "ScrapedTag", stored_id: null, name: "New" },
          ],
        }),
        url
      )
    ).toEqual({
      title: "A Story",
      author: "Someone",
      date: "2020-01-02",
      details: "About it",
      urls: [url],
      text: "Once upon a time.",
      tag_ids: ["12"],
    });
  });

  it("leaves out text and tags it doesn't have", () => {
    const input = storyImportInput(scraped({ text: "", tags: [] }), url);
    expect(input).not.toHaveProperty("text");
    expect(input).not.toHaveProperty("tag_ids");
  });
});
