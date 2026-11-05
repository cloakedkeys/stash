export const languageMap = new Map<string, string>([
  ["de", "Deutsch"],
  ["en", "English"],
  ["es", "Español"],
  ["fr", "Français"],
  ["it", "Italiano"],
  ["ja", "日本語"],
  ["ko", "한국어"],
  ["nl", "Nederlands"],
  ["pt", "Português"],
  ["ru", "Русский"],
  ["00", "Unknown"], // stash reserved language code
]);

// Names shown before they were corrected. Saved filters store the shown name,
// so these still have to find their language.
const oldLanguageNames = new Map<string, string>([
  ["Deutsche", "de"],
  ["日本", "ja"],
  ["한국인", "ko"],
  ["Holandés", "nl"],
]);

// Swaps an old name for the current one; anything else is returned unchanged.
export const currentLanguageName = (value: string) => {
  const code = oldLanguageNames.get(value);
  return code ? (languageMap.get(code) ?? value) : value;
};

// Turns a caption's language code into its standard short form, so that
// three-letter codes such as "jpn" or "ger" become "ja" or "de".
// Returns the code unchanged if it isn't a valid language code.
export const canonicalLanguageCode = (code: string) => {
  try {
    return Intl.getCanonicalLocales(code)[0] ?? code;
  } catch {
    return code;
  }
};

const capitalise = (s: string, locale: string) =>
  s.charAt(0).toLocaleUpperCase(locale) + s.slice(1);

// The name to show for a caption's language code, in that language.
// Falls back to the code itself when the language isn't known.
export const languageName = (code: string) => {
  const known = languageMap.get(code);
  if (known) {
    return known;
  }

  if (code.toLowerCase() === "und") {
    return languageMap.get("00")!;
  }

  const canonical = canonicalLanguageCode(code);
  const mapped = languageMap.get(canonical);
  if (mapped) {
    return mapped;
  }

  try {
    const name = new Intl.DisplayNames([canonical], {
      type: "language",
      fallback: "none",
    }).of(canonical);
    if (name) {
      return capitalise(name, canonical);
    }
  } catch {
    // not a language code the browser knows
  }

  return code;
};

// Converts a Captions filter value (a language name) to the code sent to the
// server. A value that isn't a known name, for example one typed into a link,
// is sent as it is rather than as an empty code.
export const valueToCode = (value?: string | null) => {
  if (!value) {
    return undefined;
  }

  return (
    Array.from(languageMap.keys()).find((v) => {
      return languageMap.get(v) === value;
    }) ??
    oldLanguageNames.get(value) ??
    value
  );
};
