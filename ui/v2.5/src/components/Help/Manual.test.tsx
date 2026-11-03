import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Manual } from "./Manual";
import { scrollToTopic } from "./scrollToTopic";

const modal = vi.hoisted(() => ({
  onShow: undefined as (() => void) | undefined,
}));

vi.mock("./scrollToTopic", () => ({ scrollToTopic: vi.fn() }));
vi.mock("../Shared/MarkdownPage", () => ({ MarkdownPage: () => null }));
vi.mock("react-bootstrap", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-bootstrap")>();
  return {
    ...original,
    Modal: Object.assign(
      ({
        children,
        onShow,
      }: React.PropsWithChildren<{ onShow?: () => void }>) => {
        modal.onShow = onShow;
        return <div>{children}</div>;
      },
      {
        Header: original.Modal.Header,
        Title: original.Modal.Title,
        Body: original.Modal.Body,
      }
    ),
  };
});

describe("opening Help on a topic (#2005)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    modal.onShow = undefined;
    vi.stubGlobal("requestAnimationFrame", vi.fn());
  });

  afterEach(() => vi.unstubAllGlobals());

  it.each([
    undefined,
    true,
    false,
  ])("scrolls after showing (animation=%s)", (animation) => {
    for (const topic of ["Identify.md", "Tasks.md"]) {
      vi.clearAllMocks();
      renderToStaticMarkup(
        <IntlProvider locale="en" messages={{ "scene_upload.title": "Upload" }}>
          <Manual
            show
            animation={animation}
            onClose={() => {}}
            defaultActiveTab={topic}
          />
        </IntlProvider>
      );
      expect(scrollToTopic).not.toHaveBeenCalled();
      expect(modal.onShow).toBeTypeOf("function");
      modal.onShow?.();
      expect(scrollToTopic).not.toHaveBeenCalled();
      expect(requestAnimationFrame).toHaveBeenCalledOnce();
      vi.mocked(requestAnimationFrame).mock.calls[0][0](0);
      expect(scrollToTopic).toHaveBeenCalledOnce();
    }
  });

  it("keeps the topic list visible when opening general Help", () => {
    renderToStaticMarkup(
      <IntlProvider locale="en" messages={{ "scene_upload.title": "Upload" }}>
        <Manual show onClose={() => {}} />
      </IntlProvider>
    );
    modal.onShow?.();
    expect(requestAnimationFrame).not.toHaveBeenCalled();
    expect(scrollToTopic).not.toHaveBeenCalled();
  });
});
