import { createIntl } from "react-intl";
import { describe, expect, it } from "vitest";
import { CriterionModifier, FilterMode } from "src/core/generated-graphql";
import { ListFilterModel } from "../filter";
import { UnsupportedCriterionOption } from "./criterion";

const intl = createIntl({
  locale: "en",
  messages: {
    "criterion.unsupported": "Unsupported criterion: {type}",
    "criterion_modifier.excludes": "excludes",
    "criterion_modifier.is_null": "is missing",
    "criterion_modifier.format_string":
      "{criterion} {modifierString} {valueString}",
  },
  onError: (error) => {
    throw error;
  },
});

describe("unsupported criterion labels", () => {
  it("renders a parent-tag filter decoded in a content list", () => {
    const value = {
      items: [{ id: "1", label: "TagZ" }],
      excluded: [],
      depth: 0,
    };
    const tags = new ListFilterModel(FilterMode.Tags);
    tags.configureFromDecodedParams({
      sortby: "name",
      q: "TAG1",
      c: [
        JSON.stringify({
          type: "name",
          modifier: CriterionModifier.Excludes,
          value: "TagX, TagY",
        }),
        JSON.stringify({
          type: "parents",
          modifier: CriterionModifier.Excludes,
          value,
        }),
      ],
    });

    const scenes = new ListFilterModel(FilterMode.Scenes);
    scenes.configureFromQueryString(tags.makeQueryParameters());
    const [parent] = scenes.criteriaFor("parents");

    expect(parent.getLabel(intl)).toBe(
      `Unsupported criterion: parents excludes ${JSON.stringify(value)}`
    );
    expect(scenes.makeFilter()).not.toHaveProperty("parents");
    expect(parent.toQueryParams()).toEqual({
      type: "parents",
      modifier: CriterionModifier.Excludes,
      value,
    });
  });

  it.each([
    { value: "TagZ", label: "TagZ" },
    {
      value: [{ id: "1", label: "TagZ" }],
      label: '[{"id":"1","label":"TagZ"}]',
    },
    { value: { value: 10, value2: 20 }, label: '{"value":10,"value2":20}' },
    { value: true, label: "true" },
  ])("formats $value as text", ({ value, label }) => {
    const criterion = new UnsupportedCriterionOption("unknown").makeCriterion();
    criterion.fromDecodedParams({ modifier: CriterionModifier.Excludes, value });

    expect(criterion.getLabel(intl)).toBe(
      `Unsupported criterion: unknown excludes ${label}`
    );
  });

  it("omits the value for a null modifier", () => {
    const criterion = new UnsupportedCriterionOption("unknown").makeCriterion();
    criterion.fromDecodedParams({
      modifier: CriterionModifier.IsNull,
      value: { items: [], excluded: [], depth: 0 },
    });

    expect(criterion.getLabel(intl)).toBe(
      "Unsupported criterion: unknown is missing "
    );
  });
});
