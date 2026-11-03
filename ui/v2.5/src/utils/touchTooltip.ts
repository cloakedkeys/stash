// On a touch screen a hint fades out a few seconds after it shows (see
// index.scss). A tapped button keeps its hint open, so tapping it again, for
// example to flip the sort direction, only changes the hint's text, and it
// would stay faded. Each tap restarts the fade of the hints on screen, so the
// new text shows long enough to read (#1953).
export function restartTooltipFades(doc: Document = document) {
  doc.querySelectorAll<HTMLElement>(".tooltip.show").forEach((el) => {
    el.style.animation = "none";
    // reading the layout makes the browser drop the old animation
    void el.offsetWidth;
    el.style.animation = "";
  });
}

export function installTouchTooltipFade(win: Window = window) {
  if (!win.matchMedia?.("(hover: none)").matches) return;
  // after the tap's own update, so a hint whose text changed is restarted
  win.document.addEventListener("click", () =>
    win.requestAnimationFrame(() => restartTooltipFades(win.document))
  );
}
