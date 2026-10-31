import React from "react";
import { useApolloClient } from "@apollo/client";
import { components } from "react-select";
import * as GQL from "src/core/generated-graphql";
import { FilterSelectComponent, toOption } from "../Shared/FilterSelect";

export type VoiceActorOption = { id: string; name: string };

export const VoiceActorSelect: React.FC<{
  values: VoiceActorOption[];
  onSelect: (items: VoiceActorOption[]) => void;
}> = ({ values, onSelect }) => {
  const client = useApolloClient();
  return (
    <FilterSelectComponent<VoiceActorOption, true>
      isMulti
      values={values}
      onSelect={onSelect}
      components={{
        Option: (props) => (
          <components.Option {...props}>
            {props.data.object.name}
          </components.Option>
        ),
        MultiValueLabel: (props) => (
          <components.MultiValueLabel {...props}>
            {props.data.object.name}
          </components.MultiValueLabel>
        ),
      }}
      loadOptions={async (q) => {
        const result = await client.query<
          GQL.FindVoiceActorsQuery,
          GQL.FindVoiceActorsQueryVariables
        >({
          query: GQL.FindVoiceActorsDocument,
          variables: { filter: { q, per_page: 40 } },
          fetchPolicy: "network-only",
        });
        return result.data.findVoiceActors.voice_actors.map(toOption);
      }}
    />
  );
};
