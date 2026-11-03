import { describe, expect, it } from "vitest";
import { scraperSiteLink } from "./scraperSiteLink";

describe("scraperSiteLink", () => {
  it("links a full address to its site", () => {
    expect(scraperSiteLink("https://example.com/scenes/")).toEqual({
      text: "https://example.com/scenes/",
      href: "https://example.com",
    });
  });

  it("assumes https for an address without one", () => {
    expect(scraperSiteLink("example.com/scenes/")).toEqual({
      text: "https://example.com/scenes/",
      href: "https://example.com",
    });
  });

  it("links an address that matches both http and https", () => {
    expect(scraperSiteLink("://literotica.com/s/")).toEqual({
      text: "https://literotica.com/s/",
      href: "https://literotica.com",
    });
  });

  it("links an address that matches any subdomain", () => {
    expect(scraperSiteLink(".literotica.com/s/")).toEqual({
      text: "https://*.literotica.com/s/",
      href: "https://literotica.com",
    });
  });

  it("leaves text that isn't an address unlinked", () => {
    expect(scraperSiteLink("https://exa mple.com/")).toEqual({
      text: "https://exa mple.com/",
    });
  });
});
