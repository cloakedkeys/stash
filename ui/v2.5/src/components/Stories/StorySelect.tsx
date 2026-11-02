import React from "react";
import {
  OptionProps,
  components as reactSelectComponents,
  SingleValueProps,
} from "react-select";
import { useIntl } from "react-intl";

import * as GQL from "src/core/generated-graphql";
import { queryFindStories } from "src/core/StashService";
import { useConfigurationContext } from "src/hooks/Config";
import { defaultMaxOptionsShown } from "src/core/config";
import { ListFilterModel } from "src/models/list-filter/filter";
import { objectTitle } from "src/core/files";
import {
  FilterSelectComponent,
  IFilterProps,
  IFilterValueProps,
  Option as SelectOption,
  toOption,
} from "../Shared/FilterSelect";
import { TruncatedText } from "../Shared/TruncatedText";

export type Story = Pick<GQL.Story, "id" | "title" | "author"> & {
  files: Pick<GQL.BasicFile, "path">[];
};

type Option = SelectOption<Story>;

// StorySelect picks one story, found by its title, author or file path.
export const StorySelect: React.FC<
  IFilterProps & IFilterValueProps<Story> & { excludeIds?: string[] }
> = (props) => {
  const { configuration } = useConfigurationContext();
  const intl = useIntl();
  const maxOptionsShown =
    configuration?.ui.maxOptionsShown ?? defaultMaxOptionsShown;

  async function loadStories(input: string): Promise<Option[]> {
    const filter = new ListFilterModel(GQL.FilterMode.Stories);
    filter.currentPage = 1;
    filter.itemsPerPage = maxOptionsShown;
    filter.sortBy = "title";
    filter.sortDirection = GQL.SortDirectionEnum.Asc;
    filter.searchTerm = input;

    const query = await queryFindStories(filter);
    const exclude = props.excludeIds ?? [];
    return query.data.findStories.stories
      .filter((s) => !exclude.includes(s.id))
      .map(toOption);
  }

  const StoryOption: React.FC<OptionProps<Option, boolean>> = (optionProps) => {
    const { object } = optionProps.data;
    const title = objectTitle(object);
    const path = object.files[0]?.path;

    return (
      <reactSelectComponents.Option {...optionProps}>
        <span className="story-select-option">
          <TruncatedText text={title} lineCount={1} />
          {object.author && (
            <span className="story-select-author">{object.author}</span>
          )}
          {path && <span className="story-select-path">{`(${path})`}</span>}
        </span>
      </reactSelectComponents.Option>
    );
  };

  const StoryValueLabel: React.FC<SingleValueProps<Option, boolean>> = (
    optionProps
  ) => (
    <reactSelectComponents.SingleValue {...optionProps}>
      {objectTitle(optionProps.data.object)}
    </reactSelectComponents.SingleValue>
  );

  return (
    <FilterSelectComponent<Story, boolean>
      {...props}
      className="story-select"
      loadOptions={loadStories}
      components={{ Option: StoryOption, SingleValue: StoryValueLabel }}
      isMulti={false}
      placeholder={intl.formatMessage(
        { id: "actions.select_entity" },
        { entityType: intl.formatMessage({ id: "story" }) }
      )}
      closeMenuOnSelect
    />
  );
};
