import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";
import messages from "src/locales/en-GB.json";
import {
  ChangeButtonSetting,
  ConstantSetting,
  Setting,
  SettingModal,
} from "./Inputs";
import { SettingSection } from "./SettingSection";
import { SettingDescription, shortLinkText } from "./SettingDescription";

vi.mock("./context", () => ({
  useSettings: () => ({ advancedMode: true }),
  useSettingsOptional: () => ({ advancedMode: true }),
}));
vi.mock("src/utils/form", () => ({ NumberField: () => null }));
// A portal has no server markup; expose the actual dialog body for this test.
vi.mock("react-bootstrap", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-bootstrap")>();
  return {
    ...original,
    Modal: Object.assign(
      ({ children }: React.PropsWithChildren) => <div>{children}</div>,
      {
        Header: original.Modal.Header,
        Body: original.Modal.Body,
        Footer: original.Modal.Footer,
      }
    ),
  };
});

function render(children: React.ReactNode) {
  return renderToStaticMarkup(
    <IntlProvider
      locale="en"
      messages={{
        description: messages.config.ui.custom_locales.description,
      }}
    >
      {children}
    </IntlProvider>
  );
}

const masterURL =
  "https://github.com/stashapp/stash/blob/develop/ui/v2.5/src/locales/en-GB.json";

describe("Settings descriptions (#1949)", () => {
  it.each([
    ["row", <Setting key="row" subHeadingID="description" />],
    [
      "edit button",
      <ChangeButtonSetting
        key="edit"
        subHeadingID="description"
        onChange={() => {}}
      />,
    ],
    ["constant", <ConstantSetting key="constant" subHeadingID="description" />],
    ["section", <SettingSection key="section" subHeadingID="description" />],
    [
      "dialog",
      <SettingModal
        key="dialog"
        subHeadingID="description"
        value=""
        close={() => {}}
        renderField={() => <input />}
      />,
    ],
  ])("links the master list in a %s", (_name, component) => {
    const html = render(component);
    expect(html).toContain('href="' + masterURL + '"');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
    expect(html).toContain(">github.com/…/en-GB.json</a>");
    expect(html).toContain(" for the master list. Page must be reloaded");
  });

  it("shows a short link text, with the full address as its title", () => {
    expect(shortLinkText(new URL(masterURL))).toBe("github.com/…/en-GB.json");
    expect(shortLinkText(new URL("https://www.example.org/"))).toBe(
      "example.org"
    );
    expect(shortLinkText(new URL("https://example.org/help"))).toBe(
      "example.org/help"
    );
    expect(shortLinkText(new URL("https://example.org/a/%E2%9C%93"))).toBe(
      "example.org/…/✓"
    );
    expect(shortLinkText(new URL("https://example.org/a/%E2"))).toBe(
      "example.org/…/%E2"
    );
    expect(
      render(<SettingDescription>{masterURL}</SettingDescription>)
    ).toContain('title="' + masterURL + '"');
  });

  it("links untranslated string descriptions too", () => {
    expect(render(<Setting subHeading={masterURL} />)).toContain(
      'href="' + masterURL + '"'
    );
  });

  it("keeps punctuation, query strings, and protocol examples intact", () => {
    const html = render(
      <SettingDescription>
        {
          "Use http:// or https:// (https://example.org/list?a=1&b=2). See http://example.net/help!"
        }
      </SettingDescription>
    );
    expect(html).toContain("Use http:// or https:// (");
    expect(html).toContain('href="https://example.org/list?a=1&amp;b=2"');
    expect(html).toContain("</a>). See ");
    expect(html).toContain('href="http://example.net/help"');
    expect(html).toContain("</a>!");
  });

  it("leaves example addresses on this computer as text", () => {
    const text =
      "a remote address (for example http://localhost:9222/json/version) or http://127.0.0.1:9222/";
    expect(render(<SettingDescription>{text}</SettingDescription>)).toBe(text);
  });

  it("preserves ordinary text and existing React content", () => {
    expect(render(<SettingDescription>Plain text</SettingDescription>)).toBe(
      "Plain text"
    );
    expect(
      render(
        <SettingDescription>
          <a href="/help">Help</a>
        </SettingDescription>
      )
    ).toBe('<a href="/help">Help</a>');
  });
});
