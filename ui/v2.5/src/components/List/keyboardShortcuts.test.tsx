import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import Mousetrap from "mousetrap";
import {
  PageListShortcutsContext,
  useListFilterShortcut,
  useListOperationShortcuts,
  usePageListShortcuts,
} from "./keyboardShortcuts";
import { ListOperationButtons } from "./ListOperationButtons";

const { effects, bindings } = vi.hoisted(() => ({
  effects: [] as (() => undefined | (() => void))[],
  bindings: new Map<string, (...events: KeyboardEvent[]) => void>(),
}));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useEffect: (effect: () => undefined | (() => void)) => effects.push(effect),
}));
vi.mock("mousetrap", () => ({
  default: {
    bind: (key: string, action: (...events: KeyboardEvent[]) => void) =>
      bindings.set(key, action),
    unbind: (key: string) => bindings.delete(key),
  },
}));
vi.mock("../Shared/Icon", () => ({ Icon: () => null }));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanModify: () => true,
  useCanAdminister: () => true,
}));

// Run effects explicitly so these binding/cleanup tests need no browser or DOM.
function mount(element: React.ReactElement) {
  renderToStaticMarkup(element);
  const cleanups = effects.splice(0).map((effect) => effect());
  return () => {
    for (const cleanup of cleanups) cleanup?.();
  };
}

beforeEach(() => {
  effects.length = 0;
  bindings.clear();
});

it("keeps page actions before selection, after deselection, and after list unmount", () => {
  const pageEdit = vi.fn();
  const pageDelete = vi.fn();
  const favorite = vi.fn();
  const listEdit = vi.fn();
  const listDelete = vi.fn();
  let page!: React.ContextType<typeof PageListShortcutsContext>;
  function Page() {
    page = usePageListShortcuts({
      onEdit: pageEdit,
      onDelete: pageDelete,
      onFavorite: favorite,
    });
    return null;
  }
  function List({ selected }: { selected: boolean }) {
    useListOperationShortcuts({
      hasSelection: selected,
      onEdit: listEdit,
      onDelete: listDelete,
    });
    return null;
  }
  const unmountPage = mount(<Page />);
  function mountList(selected: boolean) {
    return mount(
      <PageListShortcutsContext.Provider value={page}>
        <List selected={selected} />
      </PageListShortcutsContext.Provider>
    );
  }
  const press = () => {
    bindings.get("e")!();
    bindings.get("d d")!();
  };
  let unmountList = mountList(false);
  press();
  expect(pageEdit).toHaveBeenCalledTimes(1);
  expect(pageDelete).toHaveBeenCalledTimes(1);
  unmountList();
  unmountList = mountList(true);
  press();
  expect(listEdit).toHaveBeenCalledTimes(1);
  expect(listDelete).toHaveBeenCalledTimes(1);
  expect(pageEdit).toHaveBeenCalledTimes(1);
  expect(pageDelete).toHaveBeenCalledTimes(1);
  unmountList();
  unmountList = mountList(false);
  press();
  expect(pageEdit).toHaveBeenCalledTimes(2);
  expect(pageDelete).toHaveBeenCalledTimes(2);
  unmountList();
  press();
  expect(pageEdit).toHaveBeenCalledTimes(3);
  expect(pageDelete).toHaveBeenCalledTimes(3);
  bindings.get("f")!();
  expect(favorite).toHaveBeenCalledTimes(1);
  expect(page?.favorite).toBe(true);
  unmountPage();
  expect(bindings.size).toBe(0);
});

it("does not let toolbar mounting or cleanup replace page bindings", () => {
  const edit = vi.fn();
  const remove = vi.fn();
  Mousetrap.bind("e", edit);
  Mousetrap.bind("d d", remove);
  for (const selected of [false, true, false]) {
    const unmount = mount(
      <IntlProvider locale="en" messages={{}} onError={vi.fn()}>
        <ListOperationButtons itemsSelected={selected} />
      </IntlProvider>
    );
    bindings.get("e")!();
    bindings.get("d d")!();
    unmount();
    expect(bindings.get("e")).toBe(edit);
    expect(bindings.get("d d")).toBe(remove);
  }
  expect(edit).toHaveBeenCalledTimes(3);
  expect(remove).toHaveBeenCalledTimes(3);
});

it("keeps standalone list shortcuts selection-only", () => {
  const edit = vi.fn();
  const remove = vi.fn();
  function List({ selected }: { selected: boolean }) {
    useListOperationShortcuts({
      hasSelection: selected,
      onEdit: edit,
      onDelete: remove,
    });
    return null;
  }
  for (const selected of [false, true, false]) {
    const unmount = mount(<List selected={selected} />);
    bindings.get("e")!();
    bindings.get("d d")!();
    unmount();
    expect(bindings.size).toBe(0);
  }
  expect(edit).toHaveBeenCalledTimes(1);
  expect(remove).toHaveBeenCalledTimes(1);
});

it("reserves f for favourite on detail pages and filters elsewhere", () => {
  const filter = vi.fn();
  const favorite = vi.fn();
  const event = { preventDefault: vi.fn() } as unknown as KeyboardEvent;
  const selection = { current: undefined };
  function List() {
    useListFilterShortcut(filter);
    return null;
  }
  Mousetrap.bind("f", favorite);
  const unmount = mount(
    <PageListShortcutsContext.Provider value={{ selection, favorite: true }}>
      <List />
    </PageListShortcutsContext.Provider>
  );
  bindings.get("f")!(event);
  unmount();
  expect(favorite).toHaveBeenCalledTimes(1);
  expect(filter).toHaveBeenCalledTimes(0);
  expect(bindings.get("f")).toBe(favorite);

  for (const page of [undefined, { selection, favorite: false }]) {
    const unmountList = mount(
      <PageListShortcutsContext.Provider value={page}>
        <List />
      </PageListShortcutsContext.Provider>
    );
    bindings.get("f")!(event);
    unmountList();
  }
  expect(filter).toHaveBeenCalledTimes(2);
  expect(event.preventDefault).toHaveBeenCalledTimes(2);
});
