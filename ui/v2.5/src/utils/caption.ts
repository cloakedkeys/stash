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
