import type { ComponentProps, ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import type { BooleanSetting } from "../Inputs";
import { SettingsInterfacePanel } from "./SettingsInterfacePanel";

const page = vi.hoisted(() => ({
  ui: {} as { showChildGroupContent?: boolean },
  setting: undefined as ComponentProps<typeof BooleanSetting> | undefined,
  save: vi.fn(),
}));
vi.mock("../context", () => ({
  useSettings: () => ({ interface: {}, ui: page.ui, saveUI: page.save }),
}));
vi.mock("../Inputs", () => ({
  BooleanSetting: (props: ComponentProps<typeof BooleanSetting>) => {
    if (props.id === "show-child-group-content") page.setting = props;
    return null;
  },
  ModalSetting: () => null,
  NumberSetting: () => null,
  SelectSetting: () => null,
  StringSetting: () => null,
}));
vi.mock("../SettingSection", () => ({
  SettingSection: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("src/hooks/CurrentUser", () => ({ useIsAdmin: () => false }));
vi.mock("src/core/StashService", () => ({ useFindSavedFilters: () => ({}) }));
vi.mock("src/components/Shared/DurationInput", () => ({
  DurationInput: () => null,
}));
vi.mock("src/components/Shared/PercentInput", () => ({
  PercentInput: () => null,
}));
vi.mock("./CheckboxGroup", () => ({ CheckboxGroup: () => null }));
vi.mock("src/hooks/Interactive/context", async () => ({
  InteractiveContext: (await import("react")).createContext({
    interactive: {},
  }),
  ConnectionState: {},
  connectionStateLabel: () => "",
}));
vi.mock("react-intl", () => ({
  FormattedMessage: () => null,
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
it("offers an off-by-default setting and saves the group content preference", () => {
  renderToStaticMarkup(<SettingsInterfacePanel />);
  expect(page.setting).toBeDefined();
  expect(page.setting!.checked).toBe(false);
  page.setting!.onChange!(true);
  expect(page.save).toHaveBeenCalledWith({ showChildGroupContent: true });
  page.ui = { showChildGroupContent: true };
  renderToStaticMarkup(<SettingsInterfacePanel />);
  expect(page.setting!.checked).toBe(true);
  page.setting!.onChange!(false);
  expect(page.save).toHaveBeenLastCalledWith({ showChildGroupContent: false });
});
