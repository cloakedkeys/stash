import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SettingsTasksPanel } from "./SettingsTasksPanel";

const navigation = vi.hoisted(() => ({
  action: "PUSH",
  search: "?tab=tasks",
  scrollIntoView: vi.fn(),
  effects: [] as (() => void)[],
}));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useEffect: (effect: () => void) => navigation.effects.push(effect),
  useRef: () => ({ current: { scrollIntoView: navigation.scrollIntoView } }),
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({ action: navigation.action }),
  useLocation: () => ({ key: "tasks", search: navigation.search }),
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("./LibraryTasks", () => ({ LibraryTasks: () => null }));
vi.mock("./DataManagementTasks", () => ({ DataManagementTasks: () => null }));
vi.mock("./PluginTasks", () => ({ PluginTasks: () => null }));
vi.mock("./JobTable", () => ({ JobTable: () => null }));
vi.mock("src/components/Shared/LoadingIndicator", () => ({
  LoadingIndicator: () => null,
}));

beforeEach(() => {
  navigation.action = "PUSH";
  navigation.search = "?tab=tasks";
  navigation.effects = [];
  navigation.scrollIntoView.mockClear();
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    callback(0);
    return 1;
  });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
});

afterEach(() => vi.unstubAllGlobals());

function render() {
  renderToStaticMarkup(<SettingsTasksPanel />);
  for (const effect of navigation.effects.splice(0)) effect();
}

it("brings the queue into view after navigating to Tasks", () => {
  render();
  expect(navigation.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
});

it.each(["POP", "REPLACE"])("preserves scroll restoration on %s", (action) => {
  navigation.action = action;
  render();
  expect(navigation.scrollIntoView).not.toHaveBeenCalled();
});

it("does not scroll to the queue while another settings tab is open", () => {
  navigation.search = "?tab=system";
  render();
  expect(navigation.scrollIntoView).not.toHaveBeenCalled();
});
