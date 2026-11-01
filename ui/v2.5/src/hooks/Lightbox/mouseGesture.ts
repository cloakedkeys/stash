interface MousePoint {
  pageX: number;
  pageY: number;
  timeStamp: number;
}

const CLICK_MAX_DURATION = 200;
const DRAG_THRESHOLD = 5;

// Keep the press separate from the last pan position. Once a gesture has
// crossed the threshold, returning to the press cannot turn it into a click.
export function createMouseGesture(press: MousePoint) {
  let previous = press;
  let dragged = false;

  function move(point: MousePoint) {
    dragged ||=
      Math.hypot(point.pageX - press.pageX, point.pageY - press.pageY) >
      DRAG_THRESHOLD;
    const delta = [point.pageX - previous.pageX, point.pageY - previous.pageY];
    previous = point;
    return delta;
  }

  return {
    move,
    isClick(release: MousePoint) {
      move(release);
      return (
        !dragged && release.timeStamp - press.timeStamp <= CLICK_MAX_DURATION
      );
    },
    get dragged() {
      return dragged;
    },
  };
}
