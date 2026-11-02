import Countries from "i18n-iso-countries";
import { IntlShape } from "react-intl";
import { getLocaleCode } from "src/locales";

const countryCodes = Countries.getAlpha2Codes();
const localeCollators = new Map<string, Intl.Collator>();

const getCollator = (locale: string) => {
  let collator = localeCollators.get(locale);
  if (!collator) {
    try {
      collator = new Intl.Collator(locale);
    } catch {
      collator = new Intl.Collator("en");
    }
    localeCollators.set(locale, collator);
  }
  return collator;
};

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

  const code = Countries.toAlpha2(iso);
  if (!code || !Object.hasOwn(countryCodes, code)) {
    return;
  }

  const name =
    Countries.getName(code, getLocaleCode(locale)) ??
    Countries.getName(code, "en");
  return getCountryName(code, name, intl);
};

export const getCountries = (locale: string = "en", intl?: IntlShape) => {
  const localeCode = getLocaleCode(locale);
  const countries = Countries.getNames(
    Countries.langs().includes(localeCode) ? localeCode : "en"
  );
  const collator = getCollator(locale);
  return Object.entries(countries)
    .map(([code, name]) => ({
      label: getCountryName(code, name, intl) ?? name,
      value: code,
    }))
    .sort((a, b) => collator.compare(a.label, b.label));
};
