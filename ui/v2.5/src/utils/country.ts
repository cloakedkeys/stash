import Countries from "i18n-iso-countries";
import { IntlShape } from "react-intl";
import { getLocaleCode } from "src/locales";

const getCountryName = (
  code: string,
  name: string | undefined,
  intl?: IntlShape
) => {
  const id = `country_names.${code.toUpperCase()}`;
  return intl?.messages[id] ? intl.formatMessage({ id }) : name;
};

export const getCountryByISO = (
  iso: string | null | undefined,
  locale: string = "en",
  intl?: IntlShape
): string | undefined => {
  if (!iso) return;

  const ret = Countries.getName(iso, getLocaleCode(locale));
  if (ret) {
    return getCountryName(iso, ret, intl);
  }

  // fallback to english if locale is not en
  if (locale !== "en") {
    return getCountryName(iso, Countries.getName(iso, "en"), intl);
  }
};

export const getCountries = (locale: string = "en", intl?: IntlShape) => {
  let countries = Countries.getNames(getLocaleCode(locale));

  if (!countries.length) {
    countries = Countries.getNames("en");
  }

  return Object.entries(countries).map(([code, name]) => ({
    label: getCountryName(code, name, intl) ?? name,
    value: code,
  }));
};
