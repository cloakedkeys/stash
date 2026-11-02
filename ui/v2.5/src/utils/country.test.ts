import { beforeAll, describe, expect, it } from "vitest";
import Countries from "i18n-iso-countries";
import { createIntl } from "react-intl";
import { CriterionModifier } from "src/core/generated-graphql";
import { CountryCriterion } from "src/models/list-filter/criteria/country";
import { getLocaleCode, localeCountries, registerCountry } from "src/locales";
import flattenMessages from "./flattenMessages";
import { getCountries, getCountryByISO } from "./country";

beforeAll(async () => {
  await Promise.all(Object.keys(localeCountries).map(registerCountry));
});

const messages = flattenMessages({
  country_names: { TW: "Custom country name" },
  country: "Country/Region",
  criterion_modifier: {
    equals: "equals",
    not_equals: "does not equal",
    includes: "includes",
    format_string: "{criterion} {modifierString} {valueString}",
  },
});

const intl = createIntl({ locale: "en-GB", messages });

const expectedNames = [
  ["TW", "Taiwan"],
  ["HK", "Hong Kong"],
  ["MO", "Macao"],
  ["PS", "Palestine"],
  ["US", "United States"],
  ["BA", "Bosnia & Herzegovina"],
];

const expectSorted = (
  countries: ReturnType<typeof getCountries>,
  locale: string
) => {
  const collator = new Intl.Collator(locale);
  for (let i = 1; i < countries.length; i++) {
    expect(
      collator.compare(countries[i - 1].label, countries[i].label)
    ).toBeLessThanOrEqual(0);
  }
};

describe("country names", () => {
  it.each(
    Object.keys(localeCountries)
  )("ships all 250 codes in the %s file", async (locale) => {
    const { default: data } = await localeCountries[locale]();
    expect(data.locale).toBe(locale);
    expect(Object.keys(data.countries).sort()).toEqual(
      Object.keys(Countries.getAlpha2Codes()).sort()
    );
    expect(Object.keys(data.countries)).toHaveLength(250);
  });

  it("uses the stored short name for Hong Kong in Japanese", () => {
    expect(getCountryByISO("HK", "ja-JP")).toBe("香港");
    expect(getCountries("ja-JP")).toContainEqual({
      label: "香港",
      value: "HK",
    });
  });

  it.each(expectedNames)("uses the CLDR name for %s", (code, name) => {
    const defaults = createIntl({ locale: "en-GB", messages: {} });
    expect(getCountryByISO(code, defaults.locale, defaults)).toBe(name);
    expect(getCountries(defaults.locale, defaults)).toContainEqual({
      label: name,
      value: code,
    });
  });

  it.each(expectedNames)("allows a custom name for %s", (code) => {
    const custom = createIntl({
      locale: "en-GB",
      messages: { [`country_names.${code}`]: "Custom name" },
    });
    expect(getCountryByISO(code, custom.locale, custom)).toBe("Custom name");
    expect(getCountries(custom.locale, custom)).toContainEqual({
      label: "Custom name",
      value: code,
    });
  });

  it("uses custom names in labels", () => {
    expect(getCountryByISO("TW", intl.locale, intl)).toBe(
      "Custom country name"
    );
    expect(getCountryByISO("tw", intl.locale, intl)).toBe(
      "Custom country name"
    );
    expect(getCountryByISO("US", intl.locale, intl)).toBe("United States");
  });

  it.each(["USA", "us", "usa", "840"])("normalizes %s for display", (code) => {
    expect(getCountryByISO(code)).toBe(getCountryByISO("US"));
    const custom = createIntl({
      locale: "en-GB",
      messages: { "country_names.US": "Custom US" },
    });
    expect(getCountryByISO(code, custom.locale, custom)).toBe("Custom US");
  });

  it("uses custom names in searchable select options and sorts them", () => {
    const countries = getCountries(intl.locale, intl);
    expect(countries).toContainEqual({
      label: "Custom country name",
      value: "TW",
    });
    expectSorted(countries, intl.locale);
  });

  it.each([
    ["en-GB", "Kosovo"],
    ["fr-FR", "Kosovo"],
    ["zh-TW", "科索沃"],
    ["pt-BR", "Kosovo"],
  ])("lists all 250 codes with names sorted in %s", (locale, kosovo) => {
    const countries = getCountries(locale);
    expect(countries).toHaveLength(250);
    expect(countries.map(({ value }) => value).sort()).toEqual(
      Object.keys(Countries.getAlpha2Codes()).sort()
    );
    expect(countries).toContainEqual({
      label: kosovo,
      value: "XK",
    });
    expectSorted(countries, locale);
  });

  it("preserves locale and English fallback names", () => {
    const french = createIntl({ locale: "fr-FR", messages: {} });
    expect(getCountryByISO("US", french.locale, french)).toBe("États-Unis");
    expect(getCountries(french.locale, french)).toContainEqual({
      label: "États-Unis",
      value: "US",
    });
    expect(getCountryByISO("US", "xx", french)).toBe("United States");
    expect(getCountries("xx")).toContainEqual({
      label: "United States",
      value: "US",
    });
    const customFrench = createIntl({ locale: "fr-FR", messages });
    expect(getCountryByISO("TW", customFrench.locale, customFrench)).toBe(
      "Custom country name"
    );
    expectSorted(
      getCountries(customFrench.locale, customFrench),
      customFrench.locale
    );
  });

  it("falls back to English for a missing name in a registered language", () => {
    Countries.registerLocale({
      locale: "zz",
      countries: { TW: "Local Taiwan" },
    });
    expect(getCountryByISO("US", "zz")).toBe("United States");
    expect(getCountries("zz")).toEqual([
      { label: "Local Taiwan", value: "TW" },
    ]);
  });

  it("keeps an empty registered language instead of replacing it with English", () => {
    Countries.registerLocale({ locale: "zy", countries: {} });
    expect(getCountries("zy")).toEqual([]);
  });

  it("uses the country locale mapping for regional language variants", () => {
    expect(getCountryByISO("US", "zh-TW")).toBe("美國");
    expect(getCountryByISO("US", "zh-CN")).toBe("美国");
    expect(getCountryByISO("CZ", "pt-BR")).toBe("Tchéquia");
    expect(getCountryByISO("CZ", "pt-PT")).toBe("Tchéquia");
    expect(getLocaleCode("zh-TW")).toBe("tw");
    expect(getLocaleCode("zh-CN")).toBe("zh");
  });

  it.each([
    null,
    undefined,
    "",
    "ZZ",
    "QQ",
    "USA ",
    "United States",
    "Custom country",
    "!@",
  ])("returns undefined without throwing for %s", (country) => {
    expect(() => getCountryByISO(country, intl.locale, intl)).not.toThrow();
    expect(getCountryByISO(country, intl.locale, intl)).toBeUndefined();
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
    expect(criterion.getLabel(intl)).toBe("Country/Region includes TW");
  });
});
