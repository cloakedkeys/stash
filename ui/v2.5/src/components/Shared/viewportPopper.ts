// Popover positioning rules that keep a popover inside the browser window, rather
// than inside whatever element it happens to be nested in. Use with the overlay's
// `flip` prop, which turns on moving it to the opposite side when there's no room.
export const viewportModifiers = [
  {
    name: "flip",
    enabled: true,
    options: { boundary: [], rootBoundary: "viewport" },
  },
  {
    name: "preventOverflow",
    enabled: true,
    options: { boundary: [], rootBoundary: "viewport" },
  },
];
