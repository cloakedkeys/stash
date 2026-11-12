import { describe, expect, it } from "vitest";
import {
  CriterionModifier,
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { DurationCriterion } from "./criteria/criterion";
import { ListFilterModel } from "./filter";

describe("group duration filter (#1583)", () => {
  it.each([
    CriterionModifier.IsNull,
    CriterionModifier.NotNull,
  ])("offers %s and sends the duration filter input", (modifier) => {
    const filter = new ListFilterModel(FilterMode.Groups);
    const criterion = filter.makeCriterion("duration") as DurationCriterion;
    expect(criterion.modifierCriterionOption().modifierOptions).toContain(
      modifier
    );
    criterion.modifier = modifier;
    filter.criteria = [criterion];

    expect(criterion.isValid()).toBe(true);
    expect(filter.makeFilter()).toEqual({
      duration: { modifier, value: 0 },
    });
    expect(criterion.toQueryParams()).toEqual({ type: "duration", modifier });

    const loaded = new ListFilterModel(FilterMode.Groups);
    loaded.configureFromQueryString(decodeURI(filter.makeQueryParameters()));
    expect(loaded.makeFilter()).toEqual(filter.makeFilter());
  });

  it("preserves an existing greater-than-one-hour URL and saved filter", () => {
    const modifier = CriterionModifier.GreaterThan;
    const encoded = { type: "duration", modifier, value: { value: 3600 } };
    const saved = {
      duration: { modifier, value: { value: 3600, value2: undefined } },
    };
    const filter = new ListFilterModel(FilterMode.Groups);
    filter.configureFromDecodedParams({ c: [JSON.stringify(encoded)] });

    const fromURL = new ListFilterModel(FilterMode.Groups);
    fromURL.configureFromQueryString(decodeURI(filter.makeQueryParameters()));
    const fromSaved = new ListFilterModel(FilterMode.Groups);
    fromSaved.configureFromSavedFilter({
      __typename: "SavedFilter",
      id: "1",
      mode: FilterMode.Groups,
      name: "Long groups",
      find_filter: null,
      object_filter: saved,
      ui_options: null,
    } satisfies SavedFilterDataFragment);

    for (const loaded of [filter, fromURL, fromSaved]) {
      expect(loaded.criteria[0].isValid()).toBe(true);
      expect(loaded.criteria[0].toQueryParams()).toEqual(encoded);
      expect(loaded.makeSavedFilter()).toEqual(saved);
      expect(loaded.makeQueryParameters()).toBe(filter.makeQueryParameters());
      expect(loaded.makeFilter()).toEqual({
        duration: { modifier, value: 3600 },
      });
    }
  });
});
