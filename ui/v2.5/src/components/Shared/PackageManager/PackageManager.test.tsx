import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";
import { AvailablePackages } from "./PackageManager";

vi.mock("src/components/Settings/Inputs", () => ({
  SettingModal: () => null,
}));
vi.mock("../Alert", () => ({ AlertModal: () => null }));

const sources = [
  { name: "Community (stable)", url: "https://example.com/stable" },
  { name: "Community (develop)", url: "https://example.com/develop" },
];

function render(packageSources = sources) {
  return renderToStaticMarkup(
    <IntlProvider
      locale="en"
      messages={{ "package_manager.add_source": "Add source" }}
      onError={() => {}}
    >
      <AvailablePackages
        sources={packageSources}
        loadSource={async () => []}
        onInstallPackages={() => {}}
        addSource={() => {}}
        editSource={() => {}}
        deleteSource={() => {}}
      />
    </IntlProvider>
  );
}

describe("available package source controls", () => {
  it.each([
    { sources },
    { sources: [] },
  ])("keeps Add source above the scrolling table with sources %j", ({
    sources: packageSources,
  }) => {
    const html = render(packageSources);
    const addSource = html.indexOf("Add source");
    const scrollContainer = html.indexOf(
      'class="package-manager-table-container"'
    );

    expect(addSource).toBeGreaterThan(-1);
    expect(addSource).toBeLessThan(scrollContainer);
    expect(html.match(/Add source/g)).toHaveLength(1);
  });

  it("gives each source its own row group for its sticky header", () => {
    const html = render();
    const groups = html.match(/<tbody>.*?<\/tbody>/g) ?? [];

    expect(groups).toHaveLength(2);
    expect(groups[0]).toContain("Community (stable)");
    expect(groups[0]).not.toContain("Community (develop)");
    expect(groups[1]).toContain("Community (develop)");
    for (const group of groups) {
      expect(group).toContain('class="package-source"');
    }
  });

  // The last column is off screen on a phone once a source's packages are
  // listed, so Edit and Delete go beside the name instead (#1948).
  it("puts Edit and Delete in the source's name cell", () => {
    const html = render();
    const row = html.match(/<tr class="package-source">.*?<\/tr>/)?.[0] ?? "";
    const cells = row.match(/<td[^>]*>.*?<\/td>/g) ?? [];
    const nameCell = cells.find((c) => c.includes("Community (stable)"));

    expect(nameCell).toContain('colSpan="3"');
    expect(nameCell).toContain('class="source-controls"');
    expect(nameCell?.match(/<button/g)).toHaveLength(2);
    expect(cells.at(-1)).toBe(nameCell);
  });
});
