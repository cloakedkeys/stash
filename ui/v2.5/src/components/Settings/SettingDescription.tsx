import React from "react";
import { ExternalLink } from "../Shared/ExternalLink";

// Short text for a link: the site and the last part of the path, such as
// "github.com/…/en-GB.json", so a long address doesn't fill a phone screen.
export function shortLinkText(url: URL) {
  const host = url.hostname.replace(/^www\./, "");
  const parts = url.pathname.split("/").filter((p) => p);
  if (parts.length === 0) return host;
  let last = parts[parts.length - 1];
  try {
    last = decodeURIComponent(last);
  } catch {
    // keep it as written
  }
  return parts.length === 1 ? `${host}/${last}` : `${host}/…/${last}`;
}

// Descriptions are translated plain text, so React does not link URLs for us.
export const SettingDescription: React.FC<React.PropsWithChildren> = ({
  children,
}) => {
  if (typeof children !== "string") return <>{children}</>;

  return (
    <>
      {children.split(/(https?:\/\/[^\s<>"']+)/gi).map((part, index) => {
        if (index % 2 === 0) return part;

        // Keep sentence punctuation outside the link.
        const href = part.replace(/[.,;:!?)\]}]+$/, "");
        let url: URL;
        try {
          url = new URL(href);
        } catch {
          return part;
        }
        // an address on this computer is an example to copy, not a page
        if (/^(localhost|127\.\d+\.\d+\.\d+|\[::1\])$/.test(url.hostname)) {
          return part;
        }

        return (
          <React.Fragment key={index}>
            <ExternalLink href={href} title={href}>
              {shortLinkText(url)}
            </ExternalLink>
            {part.slice(href.length)}
          </React.Fragment>
        );
      })}
    </>
  );
};
