import { type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ScraperMenu } from "./ScraperMenu";
import { sortPinnedScrapers } from "src/hooks/pinnedScrapers";

const state = vi.hoisted(() => ({
  ui: {} as { pinnedScrapers?: string[] },
  save: vi.fn(),
  error: vi.fn(),
  buttons: [] as { title?: string; onClick?: (e: unknown) => void }[],
  select: vi.fn(),
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { ui: state.ui } }),
}));
vi.mock("src/core/StashService", () => ({
  useConfigureUISetting: () => [state.save, { loading: false }],
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: state.error }),
}));
vi.mock("src/utils/screen", () => ({ default: { isTouch: () => true } }));
vi.mock("src/utils/focus", () => ({ default: () => [null, vi.fn()] }));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
}));
vi.mock("react-bootstrap", () => {
  const Container = ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  );
  return {
    Dropdown: Object.assign(Container, {
      Toggle: Container,
      Menu: Container,
      Item: Container,
      Divider: () => <hr />,
    }),
    Button: (props: {
      children?: ReactNode;
      title?: string;
      onClick?: (e: unknown) => void;
    }) => {
      state.buttons.push(props);
      return <button title={props.title}>{props.children}</button>;
    },
  };
});
vi.mock("./ClearableInput", () => ({ ClearableInput: () => null }));

const scrapers = [
  { id: "a", name: "Alpha" },
  { id: "b", name: "Beta" },
  { id: "c", name: "Charlie" },
];
function render() {
  state.buttons = [];
  const markup = renderToStaticMarkup(
    <ScraperMenu
      toggle="Scrape"
      stashBoxes={[
        {
          name: "Stash box",
          endpoint: "https://example.com",
          api_key: "",
          max_requests_per_minute: 0,
        },
      ]}
      scrapers={scrapers}
      onScraperClicked={state.select}
      onReloadScrapers={() => {}}
    />
  );
  return ["Stash box", "Alpha", "Beta", "Charlie"].sort(
    (a, b) => markup.indexOf(`>${a}<`) - markup.indexOf(`>${b}<`)
  );
}
async function click(title: string, index = 0) {
  const event = { preventDefault: vi.fn(), stopPropagation: vi.fn() };
  const button = state.buttons.filter((b) => b.title === title)[index];
  expect(button).toBeDefined();
  await button.onClick!(event);
  expect(event.preventDefault).toHaveBeenCalled();
  expect(event.stopPropagation).toHaveBeenCalled();
  expect(state.select).not.toHaveBeenCalled();
}

describe("scraper pins", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.ui = {};
    state.save.mockImplementation(async ({ variables }) => {
      // Simulate the saved UI config returned by configureUISetting.
      state.ui = { ...state.ui, [variables.key]: variables.value };
    });
  });

  it("saves pins and reads them back in pin order after stash-box sources", async () => {
    expect(render()).toEqual(["Stash box", "Alpha", "Beta", "Charlie"]);
    await click("actions.pin_scraper", 2);
    expect(state.save).toHaveBeenLastCalledWith({
      variables: { key: "pinnedScrapers", value: ["c"] },
    });
    expect(render()).toEqual(["Stash box", "Charlie", "Alpha", "Beta"]);
    await click("actions.pin_scraper", 1);
    expect(state.save).toHaveBeenLastCalledWith({
      variables: { key: "pinnedScrapers", value: ["c", "b"] },
    });
    expect(render()).toEqual(["Stash box", "Charlie", "Beta", "Alpha"]);
  });

  it("unpins without scraping and restores the original order", async () => {
    state.ui = { pinnedScrapers: ["c"] };
    expect(render()).toEqual(["Stash box", "Charlie", "Alpha", "Beta"]);
    await click("actions.unpin_scraper");
    expect(state.save).toHaveBeenLastCalledWith({
      variables: { key: "pinnedScrapers", value: [] },
    });
    expect(render()).toEqual(["Stash box", "Alpha", "Beta", "Charlie"]);
  });

  it("ignores missing IDs while keeping unpinned scrapers in their original order", () => {
    state.ui = { pinnedScrapers: ["missing", "b"] };
    expect(render()).toEqual(["Stash box", "Beta", "Alpha", "Charlie"]);
    expect(sortPinnedScrapers(scrapers, ["missing", "b"])).toEqual([
      scrapers[1],
      scrapers[0],
      scrapers[2],
    ]);
    expect(scrapers.map((s) => s.id)).toEqual(["a", "b", "c"]);
    expect(sortPinnedScrapers(scrapers)).toEqual(scrapers);
  });

  it("reports failed saves and keeps the saved order", async () => {
    state.save.mockRejectedValueOnce(new Error("Save failed"));
    render();
    await click("actions.pin_scraper", 2);
    expect(state.error).toHaveBeenCalled();
    expect(render()).toEqual(["Stash box", "Alpha", "Beta", "Charlie"]);
  });
});
