import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SceneDuplicateChecker } from "./SceneDuplicateChecker";

const page = vi.hoisted(() => ({
  search: "",
  duplicates: vi.fn(),
}));

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({
    location: { search: page.search },
    push: vi.fn(),
  }),
  Link: () => null,
}));
vi.mock("src/core/generated-graphql", async (importOriginal) => ({
  ...(await importOriginal<typeof import("src/core/generated-graphql")>()),
  useFindDuplicateScenesQuery: (options: unknown) => {
    page.duplicates(options);
    return {
      data: { findDuplicateScenes: [] },
      loading: false,
      refetch: vi.fn(),
    };
  },
  useFindScenesQuery: () => ({ data: undefined }),
}));

function render(search: string) {
  page.search = search;
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={{}} onError={() => {}}>
      <SceneDuplicateChecker />
    </IntlProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("duplicate checker duration difference", () => {
  // #1288: an ad at the start or end of a copy makes it a minute longer
  it.each([30, 60])("offers %i seconds and sends it", (seconds) => {
    const html = render(`?durationDiff=${seconds}`);
    expect(html).toMatch(new RegExp(`<option value="${seconds}" selected="">`));
    expect(page.duplicates).toHaveBeenCalledWith(
      expect.objectContaining({
        variables: expect.objectContaining({ duration_diff: seconds }),
      })
    );
  });
});
