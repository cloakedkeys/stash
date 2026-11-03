import TextUtils from "src/utils/text";

export interface IScraperSiteLink {
  text: string;
  href?: string;
}

// A scraper lists the sites it handles as parts of addresses, and some don't
// start with the site: "://example.com/s/" matches both http and https, and
// ".example.com/s/" matches any subdomain. The settings page shows each one as
// a full address linked to its site, "https://*.example.com/s/" for any
// subdomain (#1963), and leaves it as text when no site can be worked out.
export function scraperSiteLink(pattern: string): IScraperSiteLink {
  const site = pattern.replace(/^[:/.]+/, "");
  const address = TextUtils.sanitiseURL(site) ?? site;
  const anySubdomain = /^[:/]*\./.test(pattern);
  try {
    const u = new URL(address);
    const text = anySubdomain ? address.replace("://", "://*.") : address;
    return { text, href: `${u.protocol}//${u.host}` };
  } catch {
    return { text: pattern };
  }
}
