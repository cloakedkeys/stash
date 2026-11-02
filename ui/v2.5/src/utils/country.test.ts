import { describe, expect, it } from "vitest";
import Countries from "i18n-iso-countries";
import en from "i18n-iso-countries/langs/en.json";
import fr from "i18n-iso-countries/langs/fr.json";
import { createIntl } from "react-intl";
import { CriterionModifier } from "src/core/generated-graphql";
import { CountryCriterion } from "src/models/list-filter/criteria/country";
import flattenMessages from "./flattenMessages";
import { getCountries, getCountryByISO } from "./country";

Countries.registerLocale(en);
Countries.registerLocale(fr);

const messages = flattenMessages({
  country_names: { TW: "Custom country name" },
  country: "Country",
  criterion_modifier: {
    equals: "equals",
    not_equals: "does not equal",
    includes: "includes",
    format_string: "{criterion} {modifierString} {valueString}",
  },
});

const intl = createIntl({ locale: "en-GB", messages });

describe("country names", () => {
  it("keeps package defaults when there are no overrides", () => {
    const defaults = createIntl({ locale: "en-GB", messages: {} });
    expect(getCountryByISO("TW", defaults.locale, defaults)).toBe(
      Countries.getName("TW", "en")
    );
    expect(getCountries(defaults.locale, defaults)).toEqual(
      Object.entries(Countries.getNames("en")).map(([code, name]) => ({
        label: name,
        value: code,
      }))
    );
  });

  it("uses custom names in labels", () => {
    expect(getCountryByISO("TW", intl.locale, intl)).toBe(
      "Custom country name"
    );
    expect(getCountryByISO("tw", intl.locale, intl)).toBe(
      "Custom country name"
    );
    expect(getCountryByISO("US", intl.locale, intl)).toBe(
      Countries.getName("US", "en")
    );
  });

  it("uses custom names in searchable select options", () => {
    expect(getCountries(intl.locale, intl)).toContainEqual({
      label: "Custom country name",
      value: "TW",
    });
  });

  it("preserves locale and English fallback names", () => {
    const french = createIntl({ locale: "fr-FR", messages: {} });
    expect(getCountryByISO("US", french.locale, french)).toBe(
      Countries.getName("US", "fr")
    );
    expect(getCountryByISO("US", "xx", french)).toBe(
      Countries.getName("US", "en")
    );
    const customFrench = createIntl({ locale: "fr-FR", messages });
    expect(getCountryByISO("TW", customFrench.locale, customFrench)).toBe(
      "Custom country name"
    );
  });

  it("preserves missing and unrecognised countries", () => {
    for (const country of [null, undefined, "", "ZZ", "Custom country"]) {
      expect(getCountryByISO(country, intl.locale, intl)).toBeUndefined();
    }
  });
});

describe("country filters", () => {
  it.each([
    CriterionModifier.Equals,
    CriterionModifier.NotEquals,
  ])("uses the custom name for %s without changing the stored code", (modifier) => {
    const criterion = new CountryCriterion();
    criterion.modifier = modifier;
    criterion.value = "TW";
    expect(criterion.getLabel(intl)).toContain("Custom country name");
    expect(criterion.toQueryParams()).toEqual({
      type: "country",
      modifier,
      value: "TW",
    });
  });

  it("keeps text matching filters literal", () => {
    const criterion = new CountryCriterion();
    criterion.modifier = CriterionModifier.Includes;
    criterion.value = "TW";
    expect(criterion.getLabel(intl)).toBe("Country includes TW");
  });
});
