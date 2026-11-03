import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";
import {
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { ListFilterModel } from "./filter";

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));

// Lists offer 20 to 120 items a page, and at most 255 (#1954).
describe("items per page", () => {
  it("uses 255 when a web address asks for more", () => {
    const filter = new ListFilterModel(FilterMode.Scenes);
    filter.configureFromQueryString("perPage=1000");
    expect(filter.itemsPerPage).toBe(255);
  });

  it("keeps an address's page size of 255 or less", () => {
    const filter = new ListFilterModel(FilterMode.Scenes);
    filter.configureFromQueryString("perPage=200");
    expect(filter.itemsPerPage).toBe(200);
  });

  it("uses 255 when a saved filter asks for more", () => {
    const filter = new ListFilterModel(FilterMode.Scenes);
    filter.configureFromSavedFilter({
      find_filter: { per_page: 500 },
      object_filter: {},
      ui_options: {},
    } as unknown as SavedFilterDataFragment);
    expect(filter.itemsPerPage).toBe(255);
  });

  it("offers 20, 40, 60 and 120, and caps Custom at 255", () => {
    const source = readFileSync(
      new URL("../../components/List/ListFilter.tsx", import.meta.url),
      "utf8"
    );
    expect(source).toContain(
      'const PAGE_SIZE_OPTIONS = ["20", "40", "60", "120"];'
    );
    expect(source).toContain("max={MAX_PAGE_SIZE}");
    expect(source).toContain("setPageSize(limitPageSize(pp));");
  });
});
