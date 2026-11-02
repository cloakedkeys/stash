import TextUtils from "src/utils/text";

export interface IScraperSiteLink {
  text: string;
  href?: string;
}

// A scraper lists the sites it handles as parts of addresses, and some don't
// start with the site: "://example.com/s/" matches both http and https, and
// ".example.com/s/" matches any subdomain. The settings page shows each one as
// a link to its site, and leaves it as text when no site can be worked out.
// Addresses like those two are shown as written, so they don't look the same.
export function scraperSiteLink(pattern: string): IScraperSiteLink {
  const site = pattern.replace(/^[:/.]+/, "");
  const address = TextUtils.sanitiseURL(site) ?? site;
  const text = site === pattern ? address : pattern;
  try {
    const u = new URL(address);
    return { text, href: `${u.protocol}//${u.host}` };
  } catch {
    return { text: pattern };
  }
}
