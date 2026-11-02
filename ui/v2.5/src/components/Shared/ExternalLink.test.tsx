import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ExternalLink } from "./ExternalLink";

const render = (href: string) =>
  renderToStaticMarkup(<ExternalLink href={href}>text</ExternalLink>);

describe("ExternalLink", () => {
  it.each([
    "javascript:alert(1)",
    "JavaScript:alert(1)",
    " javascript:alert(1)",
    "java\tscript:alert(1)",
    "\njavascript:alert(1)",
    "\u0001javascript:alert(1)",
    "vbscript:msgbox(1)",
    "data:text/html,<script>alert(1)</script>",
  ])("shows %j as plain text", (href) => {
    expect(render(href)).toBe("<span>text</span>");
  });

  it.each([
    "https://example.com/javascript:x",
    "http://example.com",
    "file:///C:/videos/a.mp4",
    "magnet:?xt=urn:btih:abc",
    "/scenes/1",
    "example.com/javascript:",
  ])("keeps %j as a link", (href) => {
    expect(render(href)).toContain("<a ");
  });
});
