import { CriterionModifier } from "src/core/generated-graphql";
import {
  currentLanguageName,
  languageMap,
  valueToCode,
} from "src/utils/caption";
import { ModifierCriterionOption, StringCriterion } from "./criterion";

const languageStrings = Array.from(languageMap.values());

export const CaptionsCriterionOption = new ModifierCriterionOption({
  messageID: "captions",
  type: "captions",
  modifierOptions: [
    CriterionModifier.Includes,
    CriterionModifier.Excludes,
    CriterionModifier.IsNull,
    CriterionModifier.NotNull,
  ],
  defaultModifier: CriterionModifier.Includes,
  options: languageStrings,
  makeCriterion: () => new CaptionCriterion(),
});

export class CaptionCriterion extends StringCriterion {
  constructor() {
    super(CaptionsCriterionOption);
  }

  // Filters saved with an old language name show and edit as the current name.
  protected decodeValue(v: unknown) {
    super.decodeValue(typeof v === "string" ? currentLanguageName(v) : v);
  }

  public setFromSavedCriterion(criterion: unknown) {
    super.setFromSavedCriterion(criterion);
    this.value = currentLanguageName(this.value);
  }

  public toCriterionInput() {
    const value = valueToCode(this.value) ?? "";

    return {
      value,
      modifier: this.modifier,
    };
  }
}
