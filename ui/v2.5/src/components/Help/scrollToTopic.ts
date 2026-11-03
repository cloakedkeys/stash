// Below Bootstrap's lg breakpoint the Help window shows its list of topics
// above the topic's text, so choosing a topic changed text that was out of
// view (#1969). There, scroll the text into view; on wider screens the two
// sit side by side and nothing needs to move.
export const stackedQuery = "(max-width: 991.98px)";

export function scrollToTopic(
  content: Pick<HTMLElement, "scrollIntoView"> | null,
  matchMedia: (query: string) => Pick<MediaQueryList, "matches">
) {
  if (content && matchMedia(stackedQuery).matches) {
    content.scrollIntoView({ block: "start" });
  }
}
