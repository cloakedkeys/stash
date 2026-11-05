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

export const valueToCode = (value?: string | null) => {
  if (!value) {
    return undefined;
  }

  return (
    Array.from(languageMap.keys()).find((v) => {
      return languageMap.get(v) === value;
    }) ?? oldLanguageNames.get(value)
  );
};
