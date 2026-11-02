import Countries, { LocaleData } from "i18n-iso-countries";

export type NestedMessage = { [key: string]: NestedMessage | string };

export const localeCountries = {
  af: () => import("./countryNames/af.json"),
  ar: () => import("./countryNames/ar.json"),
  bg: () => import("./countryNames/bg.json"),
  bn: () => import("./countryNames/bn.json"),
  ca: () => import("./countryNames/ca.json"),
  cs: () => import("./countryNames/cs.json"),
  da: () => import("./countryNames/da.json"),
  de: () => import("./countryNames/de.json"),
  en: () => import("./countryNames/en.json"),
  es: () => import("./countryNames/es.json"),
  et: () => import("./countryNames/et.json"),
  fa: () => import("./countryNames/fa.json"),
  fi: () => import("./countryNames/fi.json"),
  fr: () => import("./countryNames/fr.json"),
  hi: () => import("./countryNames/hi.json"),
  hu: () => import("./countryNames/hu.json"),
  hr: () => import("./countryNames/hr.json"),
  id: () => import("./countryNames/id.json"),
  it: () => import("./countryNames/it.json"),
  ja: () => import("./countryNames/ja.json"),
  ko: () => import("./countryNames/ko.json"),
  lt: () => import("./countryNames/lt.json"),
  lv: () => import("./countryNames/lv.json"),
  nb: () => import("./countryNames/nb.json"),
  nl: () => import("./countryNames/nl.json"),
  nn: () => import("./countryNames/nn.json"),
  pl: () => import("./countryNames/pl.json"),
  pt: () => import("./countryNames/pt.json"),
  ro: () => import("./countryNames/ro.json"),
  ru: () => import("./countryNames/ru.json"),
  sk: () => import("./countryNames/sk.json"),
  sv: () => import("./countryNames/sv.json"),
  th: () => import("./countryNames/th.json"),
  tr: () => import("./countryNames/tr.json"),
  ur: () => import("./countryNames/ur.json"),
  uk: () => import("./countryNames/uk.json"),
  vi: () => import("./countryNames/vi.json"),
  zh: () => import("./countryNames/zh.json"),
  tw: () => import("./countryNames/zh-TW.json"),
} as { [key: string]: () => Promise<{ default: LocaleData }> };

export const getLocaleCode = (code: string) => {
  if (code === "zh-CN") return "zh";
  if (code === "zh-TW") return "tw";
  return code.slice(0, 2);
};

export async function registerCountry(locale: string) {
  const localeCode = getLocaleCode(locale);
  const countries = await localeCountries[localeCode]();
  Countries.registerLocale(countries.default);
}

export const localeLoader = {
  afZA: () => import("./af-ZA.json"),
  ar: () => import("./ar.json"),
  bgBG: () => import("./bg-BG.json"),
  bnBD: () => import("./bn-BD.json"),
  caES: () => import("./ca-ES.json"),
  csCZ: () => import("./cs-CZ.json"),
  daDK: () => import("./da-DK.json"),
  deDE: () => import("./de-DE.json"),
  enGB: () => import("./en-GB.json"),
  enUS: () => import("./en-US.json"),
  esES: () => import("./es-ES.json"),
  etEE: () => import("./et-EE.json"),
  faIR: () => import("./fa-IR.json"),
  fiFI: () => import("./fi-FI.json"),
  frFR: () => import("./fr-FR.json"),
  hiIN: () => import("./hi-IN.json"),
  hrHR: () => import("./hr-HR.json"),
  huHU: () => import("./hu-HU.json"),
  idID: () => import("./id-ID.json"),
  itIT: () => import("./it-IT.json"),
  jaJP: () => import("./ja-JP.json"),
  koKR: () => import("./ko-KR.json"),
  ltLT: () => import("./lt-LT.json"),
  lvLV: () => import("./lv-LV.json"),
  nbNO: () => import("./nb-NO.json"),
  // neNP: () => import("./ne-NP.json"),
  nnNO: () => import("./nn-NO.json"),
  nlNL: () => import("./nl-NL.json"),
  plPL: () => import("./pl-PL.json"),
  ptBR: () => import("./pt-BR.json"),
  roRO: () => import("./ro-RO.json"),
  ruRU: () => import("./ru-RU.json"),
  skSK: () => import("./sk-SK.json"),
  svSE: () => import("./sv-SE.json"),
  thTH: () => import("./th-TH.json"),
  trTR: () => import("./tr-TR.json"),
  urPK: () => import("./ur-PK.json"),
  ukUA: () => import("./uk-UA.json"),
  viVN: () => import("./vi-VN.json"),
  zhCN: () => import("./zh-CN.json"),
  zhTW: () => import("./zh-TW.json"),
} as { [key: string]: () => Promise<{ default: NestedMessage }> };

export default localeLoader;
