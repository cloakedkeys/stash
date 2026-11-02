import { createStringCriterionOption } from "./criteria/criterion";
import { ListFilterOptions } from "./filter-options";
import { DisplayMode } from "./types";
import { RatingCriterionOption } from "./criteria/rating";
import { TagsCriterionOption } from "./criteria/tags";

const defaultSortBy = "title";

// only the sorts the story query accepts
const sortByOptions = ["title", "author", "date", "rating"].map(
  ListFilterOptions.createSortBy
);
const displayModeOptions = [DisplayMode.List];
const criterionOptions = [
  createStringCriterionOption("title"),
  TagsCriterionOption,
  RatingCriterionOption,
];

export const StoryListFilterOptions = new ListFilterOptions(
  defaultSortBy,
  sortByOptions,
  displayModeOptions,
  criterionOptions
);
