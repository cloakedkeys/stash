// Below Bootstrap's lg breakpoint the Help window shows its list of topics
// above the topic's text, so choosing a topic changed text that was out of
// view (#1969). There, scroll the window's body to the text; on wider screens
// the two sit side by side and nothing needs to move.
export const stackedQuery = "(max-width: 991.98px)";

interface IBox {
  getBoundingClientRect(): { top: number };
}

interface IContent extends IBox {
  closest(selector: string): (IBox & { scrollTop: number }) | null;
}

export function scrollToTopic(
  content: IContent | null,
  matchMedia: (query: string) => Pick<MediaQueryList, "matches">
) {
  if (!content || !matchMedia(stackedQuery).matches) return;
  // only the window's own body, never the page behind it
  const body = content.closest(".modal-body");
  if (!body) return;
  body.scrollTop +=
    content.getBoundingClientRect().top - body.getBoundingClientRect().top;
}
