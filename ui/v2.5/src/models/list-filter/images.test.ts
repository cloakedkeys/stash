import { createIntl } from "react-intl";
import { describe, expect, it } from "vitest";
import {
  CriterionModifier,
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import messages from "src/locales/en-GB.json";
import { ListFilterModel } from "./filter";

function imageFilter(value: string) {
  const filter = new ListFilterModel(FilterMode.Images);
  const criterion = filter.makeCriterion("is_clip");
  criterion.setFromSavedCriterion({
    value,
    modifier: CriterionModifier.Equals,
  });
  filter.criteria = [criterion];
  return filter;
}

function expectedFilter(value: string) {
  return {
    files_filter: {
      video_file_filter: {
        format: {
          value: "^",
          modifier:
            value === "true"
              ? CriterionModifier.MatchesRegex
              : CriterionModifier.NotMatchesRegex,
        },
      },
    },
  };
}

describe("image clip filter (#1268)", () => {
  it("offers a yes/no criterion on images", () => {
    const filter = new ListFilterModel(FilterMode.Images);
    const option = filter.options.criterionOptions.find(
      (o) => o.type === "is_clip"
    );
    expect(option).toMatchObject({
      messageID: "is_clip",
      options: ["true", "false"],
      modifierOptions: [],
    });
  });

  describe.each(["true", "false"])("%s", (value) => {
    it("sends the existing video file filter", () => {
      expect(imageFilter(value).makeFilter()).toEqual(expectedFilter(value));
    });

    it("survives the URL", () => {
      const loaded = new ListFilterModel(FilterMode.Images);
      loaded.configureFromQueryString(
        decodeURI(imageFilter(value).makeQueryParameters())
      );
      expect(loaded.makeFilter()).toEqual(expectedFilter(value));
      expect(loaded.criteria[0].isValid()).toBe(true);
    });

    it("survives a saved filter", () => {
      const saved = imageFilter(value).makeSavedFilter();
      expect(saved).toEqual({
        is_clip: { value, modifier: CriterionModifier.Equals },
      });
      const loaded = new ListFilterModel(FilterMode.Images);
      loaded.configureFromSavedFilter({
        __typename: "SavedFilter",
        id: "1",
        mode: FilterMode.Images,
        name: "Clips",
        find_filter: null,
        object_filter: JSON.parse(JSON.stringify(saved)),
        ui_options: null,
      } satisfies SavedFilterDataFragment);
      expect(loaded.makeFilter()).toEqual(expectedFilter(value));
      expect(loaded.makeSavedFilter()).toEqual(saved);
    });

    it("shows a readable label", () => {
      const intl = createIntl({
        locale: "en-GB",
        messages: {
          is_clip: messages.is_clip,
          "criterion.unsupported": messages.criterion.unsupported,
          "criterion_modifier.equals": messages.criterion_modifier.equals,
          "criterion_modifier.format_string":
            messages.criterion_modifier.format_string,
        },
      });
      expect(imageFilter(value).criteria[0].getLabel(intl)).toBe(
        `Is clip is ${value}`
      );
    });

    it("keeps the folder criterion alongside the clip criterion", () => {
      const filter = imageFilter(value);
      const folder = filter.makeCriterion("folder");
      folder.setFromSavedCriterion({
        value: {
          items: [{ id: "1", label: "Pictures" }],
          excluded: [],
          depth: 0,
        },
        modifier: CriterionModifier.Includes,
      });
      filter.criteria.push(folder);
      const output = filter.makeFilter();
      expect(output).toMatchObject(expectedFilter(value));
      expect(output).toHaveProperty("AND.files_filter.parent_folder.value", [
        "1",
      ]);
    });
  });
});
