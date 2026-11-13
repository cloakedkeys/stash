import React from "react";
import * as GQL from "src/core/generated-graphql";
import { BooleanSetting } from "../Inputs";

interface IAutoTagOptions {
  options: GQL.AutoTagMetadataInput;
  setOptions: (s: GQL.AutoTagMetadataInput) => void;
}

export const AutoTagOptions: React.FC<IAutoTagOptions> = ({
  options,
  setOptions: setOptionsState,
}) => {
  const { performers, studios, tags } = options;
  const wildcard = ["*"];

  function set(v?: boolean) {
    if (v) {
      return wildcard;
    }
    return [];
  }

  function setOptions(input: Partial<GQL.AutoTagMetadataInput>) {
    setOptionsState({ ...options, ...input });
  }

  return (
    <>
      <BooleanSetting
        id="autotag-performers"
        checked={!!performers?.length}
        headingID="performers"
        onChange={(v) => setOptions({ performers: set(v) })}
      />
      <BooleanSetting
        id="autotag-longest-match-only"
        checked={!!options.longest_match_only}
        headingID="config.tasks.auto_tag.longest_match_only.heading"
        subHeadingID="config.tasks.auto_tag.longest_match_only.description"
        onChange={(v) => setOptions({ longest_match_only: v })}
      />
      <BooleanSetting
        id="autotag-studios"
        checked={!!studios?.length}
        headingID="studios"
        onChange={(v) => setOptions({ studios: set(v) })}
      />
      <BooleanSetting
        id="autotag-tags"
        checked={!!tags?.length}
        headingID="tags"
        onChange={(v) => setOptions({ tags: set(v) })}
      />
    </>
  );
};
