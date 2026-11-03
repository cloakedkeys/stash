import React from "react";
import { describe, expect, it, vi } from "vitest";
import { canExpandToast, IToast, ToastMessage } from "./Toast";

interface IElement {
  type: unknown;
  props: {
    className?: string;
    onClick?: () => void;
    children?: unknown;
  };
}

// Renders the message the way the toast does, without a DOM: ToastMessage has
// no hooks, so calling it returns the element tree it shows.
function renderMessage(toast: Pick<IToast, "variant" | "content">) {
  const onExpand = vi.fn();
  const tree = ToastMessage({
    toast,
    onExpand,
    onLinkClick: () => {},
  }) as React.ReactElement<{ children: React.ReactNode }>;
  const [text, button] = React.Children.toArray(
    tree.props.children
  ) as unknown as IElement[];
  return { onExpand, text, button };
}

describe("ToastMessage", () => {
  it("does nothing when a normal message is tapped (#1973)", () => {
    for (const variant of [undefined, "success", "warning"] as const) {
      const { onExpand, text, button } = renderMessage({
        variant,
        content: "Added Generate to job queue",
      });
      text.props.onClick?.();
      expect(onExpand).not.toHaveBeenCalled();
      expect(button).toBeUndefined();
    }
  });

  it("opens an error when its text or its button is tapped", () => {
    const { onExpand, text, button } = renderMessage({
      variant: "danger",
      content: "Something failed",
    });
    text.props.onClick?.();
    expect(onExpand).toHaveBeenCalledTimes(1);
    expect(button.props.className).toBe("expand-error-button");
    button.props.onClick?.();
    expect(onExpand).toHaveBeenCalledTimes(2);
  });
});

describe("canExpandToast", () => {
  it("opens only errors in the box titled Error", () => {
    expect(canExpandToast({ variant: "danger" })).toBe(true);
    expect(canExpandToast({})).toBe(false);
    expect(canExpandToast({ variant: "success" })).toBe(false);
    expect(canExpandToast({ variant: "warning" })).toBe(false);
  });
});
