import { beforeEach, expect, it, vi } from "vitest";
import { useSettingsReturnLocation } from "./settingsNavigation";

const router = vi.hoisted(() => ({
  location: { pathname: "/", search: "", hash: "", state: undefined },
  ref: { current: undefined as unknown },
  effects: [] as (() => void)[],
}));

vi.mock("react", () => ({
  useRef: () => router.ref,
  useLayoutEffect: (effect: () => void) => router.effects.push(effect),
}));
vi.mock("react-router-dom", () => ({
  useLocation: () => router.location,
}));

function visit(url: string) {
  const parsed = new URL(url, "http://localhost");
  router.location = {
    pathname: parsed.pathname,
    search: parsed.search,
    hash: parsed.hash,
    state: undefined,
  };
  // biome-ignore lint/correctness/useHookAtTopLevel: mocked hooks simulate a render without a browser
  const destination = useSettingsReturnLocation();
  for (const effect of router.effects.splice(0)) effect();
  return destination;
}

beforeEach(() => {
  router.ref.current = undefined;
  router.effects = [];
});

it("keeps the page before Settings through redirects and tab navigation", () => {
  visit("/scenes?page=3&sortby=date#scene-42");
  const page = router.location;
  expect(visit("/settings")).toBe(page);
  expect(visit("/settings?tab=tasks")).toBe(page);
  expect(visit("/settings?tab=interface")).toBe(page);
  expect(visit("/settings/?tab=about")).toBe(page);
  expect(page.search).toBe("?page=3&sortby=date");
  expect(page.hash).toBe("#scene-42");
});

it("updates the destination when Settings is reopened from another page", () => {
  visit("/scenes");
  visit("/settings?tab=tasks");
  visit("/performers/12");
  const page = router.location;
  expect(visit("/settings?tab=about")).toBe(page);
});

it("has no return location when the first page is Settings", () => {
  expect(visit("/settings?tab=interface")).toBeUndefined();
  expect(visit("/settings?tab=about")).toBeUndefined();
});

it("treats Settings in any case as Settings, as the router does", () => {
  expect(visit("/Settings?tab=interface")).toBeUndefined();
  expect(visit("/SETTINGS?tab=about")).toBeUndefined();
});
