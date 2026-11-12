import { CriterionModifier } from "src/core/generated-graphql";
import { BooleanCriterion, BooleanCriterionOption } from "./criterion";

export const IsClipCriterionOption = new BooleanCriterionOption(
  "is_clip",
  "is_clip",
  () => new IsClipCriterion()
);

export class IsClipCriterion extends BooleanCriterion {
  constructor() {
    super(IsClipCriterionOption);
  }

  public applyToCriterionInput(input: Record<string, unknown>) {
    input.files_filter = {
      video_file_filter: {
        format: {
          // Format is NOT NULL on video rows. Match even an empty format;
          // the negative regex uses a left join and keeps non-video files.
          value: "^",
          modifier: this.toCriterionInput()
            ? CriterionModifier.MatchesRegex
            : CriterionModifier.NotMatchesRegex,
        },
      },
    };
  }
}
