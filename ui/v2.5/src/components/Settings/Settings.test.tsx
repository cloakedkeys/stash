import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import { Settings } from "./Settings";

const navigation = vi.hoisted(() => ({
  replace: vi.fn(),
  close: undefined as (() => void) | undefined,
}));

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/settings", search: "?tab=about" }),
  useHistory: () => ({ replace: navigation.replace }),
}));
vi.mock("react-router-bootstrap", () => ({
  LinkContainer: ({ children }: React.PropsWithChildren) => children,
}));
vi.mock("react-bootstrap", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-bootstrap")>()),
  CloseButton: (props: { label: string; onClick: () => void }) => {
    navigation.close = props.onClick;
    return <button type="button" aria-label={props.label} />;
  },
}));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanAdminister: () => false,
  useCanModify: () => false,
}));
vi.mock("src/hooks/title", () => ({ useTitleProps: () => ({}) }));
vi.mock("./context", () => ({
  SettingsContext: ({ children }: React.PropsWithChildren) => children,
  useSettings: () => ({
    advancedMode: false,
    setAdvancedMode: vi.fn(),
    saveIndicator: <span data-save-status="failed" />,
  }),
}));
vi.mock("../TroubleshootingMode/useTroubleshootingMode", () => ({
  useTroubleshootingMode: () => ({ isActive: false }),
}));
vi.mock("../TroubleshootingMode/TroubleshootingModeButton", () => ({
  TroubleshootingModeButton: () => null,
}));
vi.mock("./SettingsAboutPanel", () => ({ SettingsAboutPanel: () => null }));
vi.mock("./SettingsSystemPanel", () => ({
  SettingsConfigurationPanel: () => null,
}));
vi.mock("./SettingsInterfacePanel/SettingsInterfacePanel", () => ({
  SettingsInterfacePanel: () => null,
}));
vi.mock("./SettingsLogsPanel", () => ({ SettingsLogsPanel: () => null }));
vi.mock("./Tasks/SettingsTasksPanel", () => ({
  SettingsTasksPanel: () => null,
}));
vi.mock("./SettingsPluginsPanel", () => ({ SettingsPluginsPanel: () => null }));
vi.mock("./SettingsScrapingPanel", () => ({
  SettingsScrapingPanel: () => null,
}));
vi.mock("./SettingsToolsPanel", () => ({ SettingsToolsPanel: () => null }));
vi.mock("./SettingsServicesPanel", () => ({
  SettingsServicesPanel: () => null,
}));
vi.mock("./SettingsLibraryPanel", () => ({ SettingsLibraryPanel: () => null }));
vi.mock("./SettingsSecurityPanel", () => ({
  SettingsSecurityPanel: () => null,
}));
vi.mock("../Changelog/Changelog", () => ({ default: () => null }));

beforeEach(() => {
  navigation.replace.mockClear();
  navigation.close = undefined;
});

function render(
  returnLocation?: React.ComponentProps<typeof Settings>["returnLocation"]
) {
  return renderToStaticMarkup(
    <IntlProvider
      locale="en"
      messages={{ "actions.close": "Close" }}
      onError={() => {}}
    >
      <Settings returnLocation={returnLocation} />
    </IntlProvider>
  );
}

it("closes to the saved location with its query, fragment and state", () => {
  const page = {
    pathname: "/scenes",
    search: "?page=3",
    hash: "#scene-42",
    state: { selected: "42" },
  };
  expect(render(page)).toContain('aria-label="Close"');
  expect(navigation.close).toBeTypeOf("function");
  navigation.close!();
  expect(navigation.replace).toHaveBeenCalledWith(page);
});

it("closes to Home when Settings was opened directly", () => {
  render();
  expect(navigation.close).toBeTypeOf("function");
  navigation.close!();
  expect(navigation.replace).toHaveBeenCalledWith("/");
});

it("keeps save feedback beside the Settings close button", () => {
  expect(render()).toContain(
    '<div class="settings-controls"><span data-save-status="failed"></span><button type="button" aria-label="Close"'
  );
});
