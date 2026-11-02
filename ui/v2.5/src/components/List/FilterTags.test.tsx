import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { beforeEach, expect, it, vi } from "vitest";
import { useQuery } from "@apollo/client";
import * as GQL from "src/core/generated-graphql";
import {
  Criterion,
  StringCriterion,
  StringCriterionOption,
} from "src/models/list-filter/criteria/criterion";
import {
  ChildTagsCriterionOption,
  ParentTagsCriterionOption,
  PerformerTagsCriterionOption,
  SceneTagsCriterionOption,
  TagsCriterionOption,
} from "src/models/list-filter/criteria/tags";
import {
  ParentStudiosCriterion,
  StudiosCriterion,
} from "src/models/list-filter/criteria/studios";
import {
  ContainingGroupsCriterionOption,
  GroupsCriterionOption,
  SubGroupsCriterionOption,
} from "src/models/list-filter/criteria/groups";
import { PerformersCriterion } from "src/models/list-filter/criteria/performers";
import { ScenesCriterion } from "src/models/list-filter/criteria/scenes";
import { GalleriesCriterion } from "src/models/list-filter/criteria/galleries";
import { FilterTags } from "./FilterTags";

vi.mock("@apollo/client", () => ({ useQuery: vi.fn() }));
vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useLayoutEffect: () => {},
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({
    configuration: { interface: { sfwContentMode: false } },
  }),
}));
vi.mock("src/patch", () => ({ PatchContainerComponent: () => () => null }));
vi.mock("../Shared/Icon", () => ({ Icon: () => null }));

const messages = {
  "criterion_modifier.includes": "includes",
  "criterion_modifier.includes_all": "includes all",
  "criterion_modifier.excludes": "excludes",
  "criterion_modifier.is_null": "is missing",
  "criterion_modifier.format_string": "{criterion} {modifierString} {valueString}",
  "criterion_modifier.format_string_excludes":
    "{criterion} {modifierString} {valueString} excluding {excludedString}",
  "criterion_modifier.format_string_excludes_depth":
    "{criterion} {modifierString} {valueString} excluding {excludedString} depth {depth}",
};

function renderBadge(criterion: Criterion) {
  return renderToStaticMarkup(
    <IntlProvider locale="en" messages={messages} onError={() => {}}>
      <FilterTags
        criteria={[criterion]}
        onEditCriterion={() => {}}
        onRemoveCriterion={() => {}}
        onRemoveAll={() => {}}
      />
    </IntlProvider>
  );
}

beforeEach(() => {
  vi.mocked(useQuery).mockReset();
  vi.mocked(useQuery).mockReturnValue({ data: undefined } as ReturnType<
    typeof useQuery
  >);
});

const hierarchicalCases = [
  ["tags", TagsCriterionOption],
  ["parent tags", ParentTagsCriterionOption],
  ["child tags", ChildTagsCriterionOption],
  ["scene tags", SceneTagsCriterionOption],
  ["performer tags", PerformerTagsCriterionOption],
  ["groups", GroupsCriterionOption],
  ["containing groups", ContainingGroupsCriterionOption],
  ["sub groups", SubGroupsCriterionOption],
] as const;

it.each(hierarchicalCases)("refreshes saved %s badge names by ID", (_, option) => {
  const criterion = option.makeCriterion();
  criterion.setFromSavedCriterion({
    modifier: GQL.CriterionModifier.Includes,
    value: { items: [{ id: "1", label: "Test ABC" }], excluded: [], depth: 0 },
  });
  const tags = option.inputType === "tags";
  vi.mocked(useQuery).mockReturnValue({
    data: tags
      ? { findTags: { tags: [{ id: "1", name: "Test XYZ" }] } }
      : { findGroups: { groups: [{ id: "1", name: "Test XYZ" }] } },
  } as ReturnType<typeof useQuery>);

  const markup = renderBadge(criterion);
  expect(markup).toContain("Test XYZ");
  expect(markup).not.toContain("Test ABC");
  expect(useQuery).toHaveBeenCalledWith(
    tags ? GQL.FindTagsForSelectDocument : GQL.FindGroupsForSelectDocument,
    expect.objectContaining({
      variables: { ids: ["1"] },
      fetchPolicy: "cache-and-network",
      skip: false,
    })
  );
});

it.each([
  ["performers", () => new PerformersCriterion(), "findPerformers", "performers"],
  ["studios", () => new StudiosCriterion(), "findStudios", "studios"],
] as const)(
  "refreshes included and excluded %s without changing saved values",
  (_, make, field, list) => {
    const criterion = make();
    criterion.setFromSavedCriterion({
      modifier: GQL.CriterionModifier.Includes,
      value: {
        items: [{ id: "1", label: "Old included" }],
        excluded: [{ id: "2", label: "Old excluded" }],
        depth: -1,
      },
    });
    const before = JSON.stringify(criterion.toQueryParams());
    const input = criterion.toCriterionInput();
    vi.mocked(useQuery).mockReturnValue({
      data: {
        [field]: {
          [list]: [
            { id: "2", name: "New excluded" },
            { id: "1", name: "New included" },
          ],
        },
      },
    } as ReturnType<typeof useQuery>);

    const markup = renderBadge(criterion);
    expect(markup).toContain("New included");
    expect(markup).toContain("New excluded");
    expect(markup).not.toContain("Old included");
    expect(markup).not.toContain("Old excluded");
    expect(criterion.toCriterionInput()).toEqual(input);
    expect(JSON.stringify(criterion.toQueryParams())).toBe(before);
    expect(useQuery).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ variables: { ids: ["1", "2"] } })
    );
  }
);

it("refreshes flat ID lists and keeps missing entities as saved", () => {
  const criterion = new ParentStudiosCriterion();
  criterion.setFromSavedCriterion({
    modifier: GQL.CriterionModifier.Includes,
    value: [
      { id: "1", label: "Old studio" },
      { id: "2", label: "Missing studio" },
    ],
  });
  expect(renderBadge(criterion)).toContain("Old studio");
  vi.mocked(useQuery).mockReturnValue({
    data: { findStudios: { studios: [{ id: "1", name: "Renamed studio" }] } },
  } as ReturnType<typeof useQuery>);
  const markup = renderBadge(criterion);
  expect(markup).toContain("Renamed studio, Missing studio");
  expect(markup).not.toContain("Old studio");
});

it.each([
  ["scenes", () => new ScenesCriterion(), "findScenes"],
  ["galleries", () => new GalleriesCriterion(), "findGalleries"],
] as const)(
  "refreshes %s titles and uses the file name for untitled items",
  (list, make, field) => {
    const criterion = make();
    criterion.setFromSavedCriterion({
      modifier: GQL.CriterionModifier.Includes,
      value: [
        { id: "1", label: "Old title" },
        { id: "2", label: "Old file" },
      ],
    });
    vi.mocked(useQuery).mockReturnValue({
      data: {
        [field]: {
          [list]: [
            { id: "1", title: "New title", files: [] },
            { id: "2", title: "", files: [{ path: "/library/new-file.mp4" }] },
          ],
        },
      },
    } as ReturnType<typeof useQuery>);
    expect(renderBadge(criterion)).toContain("New title, new-file.mp4");
  }
);

it("does not query for scalar criteria", () => {
  const criterion = new StringCriterion(
    new StringCriterionOption({ type: "path", messageID: "path" })
  );
  criterion.value = "literal path";
  expect(renderBadge(criterion)).toContain("literal path");
  expect(useQuery).not.toHaveBeenCalled();
});

it("skips name queries for missing-value modifiers and empty selections", () => {
  const criterion = new StudiosCriterion();
  renderBadge(criterion);
  expect(useQuery).toHaveBeenLastCalledWith(
    expect.anything(),
    expect.objectContaining({ skip: true })
  );
  criterion.value.items = [{ id: "1", label: "Old name" }];
  criterion.modifier = GQL.CriterionModifier.IsNull;
  expect(renderBadge(criterion)).not.toContain("Old name");
  expect(useQuery).toHaveBeenLastCalledWith(
    expect.anything(),
    expect.objectContaining({ skip: true })
  );
});

it("refreshes excluded-only tags and legacy performer selections", () => {
  for (const criterion of [
    ParentTagsCriterionOption.makeCriterion(),
    new PerformersCriterion(),
  ]) {
    criterion.setFromSavedCriterion({
      modifier: GQL.CriterionModifier.Excludes,
      value:
        criterion instanceof PerformersCriterion
          ? [{ id: "1", label: "Old excluded" }]
          : { items: [{ id: "1", label: "Old excluded" }], depth: 0 },
    });
    vi.mocked(useQuery).mockReturnValue({
      data:
        criterion instanceof PerformersCriterion
          ? { findPerformers: { performers: [{ id: "1", name: "New excluded" }] } }
          : { findTags: { tags: [{ id: "1", name: "New excluded" }] } },
    } as ReturnType<typeof useQuery>);
    const markup = renderBadge(criterion);
    expect(markup).toContain("excludes New excluded");
    expect(markup).not.toContain("Old excluded");
  }
});
