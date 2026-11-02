// Regenerate from ui/v2.5 with: node scripts/generate-country-names.cjs
// Use a full-ICU Node build with CLDR 48 or newer; the CLDR version is printed below.
const fs = require("node:fs");
const path = require("node:path");
const Countries = require("i18n-iso-countries");

const cldrVersion = process.versions.cldr;
if (!cldrVersion || !(Number.parseInt(cldrVersion, 10) >= 48)) {
  throw new Error(
    `CLDR 48 or newer is required; found ${cldrVersion ?? "none"}`
  );
}
console.log(`Generating country names with CLDR ${cldrVersion}`);

// Only these regions use CLDR's short forms.
const SHORT_NAME_CODES = new Set(["HK", "MO", "PS"]);
// getLocaleCode merges UI locales into these country-language keys.
const FULL_LOCALES = { tw: "zh-TW", zh: "zh-CN" };
const localesDir = path.join(__dirname, "../src/locales");
const loaders = fs.readFileSync(path.join(localesDir, "index.ts"), "utf8");
const languages = Array.from(
  loaders.matchAll(
    /^ {2}(\w+): \(\) => import\("\.\/countryNames\/([^"\n]+\.json)"\),$/gm
  ),
  ([, code, file]) => ({ code, file })
).sort((a, b) => (a.code < b.code ? -1 : a.code > b.code ? 1 : 0));
if (!languages.length || !languages.some(({ code }) => code === "en")) {
  throw new Error("Country loaders, including English, were not found");
}
const codes = Object.keys(Countries.getAlpha2Codes()).sort();
if (codes.length !== 250) {
  throw new Error(`Expected 250 country codes; found ${codes.length}`);
}

function getNames(code) {
  const locale = FULL_LOCALES[code] ?? code;
  const names = new Intl.DisplayNames([locale], {
    type: "region",
    fallback: "none",
  });
  const shortNames = new Intl.DisplayNames([locale], {
    type: "region",
    style: "short",
    fallback: "none",
  });
  return Object.fromEntries(
    codes.map((countryCode) => {
      const name = (SHORT_NAME_CODES.has(countryCode) ? shortNames : names).of(
        countryCode
      );
      if (!name || name === countryCode) {
        throw new Error(`Missing ${locale} name for ${countryCode}`);
      }
      return [countryCode, name];
    })
  );
}

const english = getNames("en");
// Validate every language before overwriting any files.
const generated = languages.map(({ code, file }) => {
  const countries = code === "en" ? english : getNames(code);
  if (code !== "en" && codes.every((key) => countries[key] === english[key])) {
    throw new Error(
      `All ${code} names are English; Node may lack ICU language data`
    );
  }
  return { file, data: { locale: code, countries } };
});
const outputDir = path.join(localesDir, "countryNames");
fs.mkdirSync(outputDir, { recursive: true });
for (const { file, data } of generated) {
  fs.writeFileSync(
    path.join(outputDir, file),
    `${JSON.stringify(data, null, 2)}\n`
  );
}
console.log(
  `Wrote ${generated.length} language files with ${codes.length} names each`
);
