import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SceneDuplicateChecker } from "./SceneDuplicateChecker";

const page = vi.hoisted(() => ({
  search: "",
  duplicates: vi.fn(),
  missingPhashes: vi.fn(),
  push: vi.fn(),
  check: undefined as
    | undefined
    | {
        id?: string;
        onChange: (e: { currentTarget: { checked: boolean } }) => void;
      },
}));

// Capture the checkbox handler while rendering the real Bootstrap control.
vi.mock("react-bootstrap", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-bootstrap")>();
  const Form = Object.assign(
    (props: React.ComponentPropsWithoutRef<"form">) => (
      <original.Form {...props} />
    ),
    original.Form,
    {
      Check: (
        props: Omit<React.ComponentProps<typeof original.Form.Check>, "as">
      ) => {
        if (props.id === "match-stash-id")
          page.check = props as typeof page.check;
        return <original.Form.Check {...props} />;
      },
    }
  );
  return { ...original, Form };
});

vi.mock("src/core/createClient", () => ({ createClient: () => ({}) }));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({
    location: { search: page.search },
    push: page.push,
  }),
  Link: () => null,
}));
// the loaded page has the scene filter row, which reads the configuration
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: { interface: {} } }),
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
  useFindScenesQuery: (options: unknown) => {
    page.missingPhashes(options);
    return { data: { findScenes: { count: 2 } } };
  },
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
  page.check = undefined;
});

// #1272: loading a shared address selects Stash ID matching and hides phash settings.
describe("duplicate checker Stash ID matching", () => {
  it("restores the mode from the address and hides irrelevant settings", () => {
    const html = render("?stashID=true&distance=4&durationDiff=60");
    expect(html).toContain('id="match-stash-id"');
    expect(html).toMatch(/id="match-stash-id"[^>]*checked=""/);
    expect(html).not.toContain("dupe_check.search_accuracy_label");
    expect(html).not.toContain("dupe_check.duration_diff");
    expect(html).not.toContain("Missing phashes");
    expect(page.missingPhashes).toHaveBeenCalledWith(
      expect.objectContaining({ skip: true })
    );
    expect(html).toContain("dupe_check.scene_filter");
    expect(page.duplicates).toHaveBeenCalledWith(
      expect.objectContaining({
        variables: expect.objectContaining({ stash_id: true }),
      })
    );
  });
});

it("keeps the mode in the address, resets pagination and restores phash settings", () => {
  render("?distance=4&durationDiff=60&page=3&size=50&unknown=kept");
  expect(page.check).toBeDefined();
  page.check!.onChange({ currentTarget: { checked: true } });
  let search = page.push.mock.calls.at(-1)![0].search as string;
  let params = new URLSearchParams(search);
  expect(params.get("stashID")).toBe("true");
  expect(params.has("page")).toBe(false);
  expect(params.get("size")).toBe("50");
  expect(params.get("unknown")).toBe("kept");
  render(search);
  page.check!.onChange({ currentTarget: { checked: false } });
  search = page.push.mock.calls.at(-1)![0].search as string;
  params = new URLSearchParams(search);
  expect(params.has("stashID")).toBe(false);
  const html = render(search);
  expect(html).toContain("dupe_check.search_accuracy_label");
  expect(html).toContain("Missing phashes");
  expect(html).toContain('<option value="4" selected="">');
  expect(html).toContain('<option value="60" selected="">');
});
