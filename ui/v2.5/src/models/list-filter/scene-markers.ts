import { PerformersCriterionOption } from "./criteria/performers";
import { MarkersScenesCriterionOption } from "./criteria/scenes";
import { SceneTagsCriterionOption, TagsCriterionOption } from "./criteria/tags";
import { ListFilterOptions } from "./filter-options";
import { CriterionType, DisplayMode } from "./types";
import {
  createDateCriterionOption,
  createMandatoryTimestampCriterionOption,
  createNullDurationCriterionOption,
  createSubFilterCriterionOption,
} from "./criteria/criterion";
import { SceneListFilterOptions } from "./scenes";

const defaultSortBy = "title";
const sortByOptions = [
  "duration",
  "title",
  "seconds",
  "scene_id",
  "random",
  "scenes_updated_at",
].map(ListFilterOptions.createSortBy);
const displayModeOptions = [DisplayMode.Grid, DisplayMode.Wall];

// Criteria of the marker's scene (#1548), sent in scene_filter. The scene
// list's own criterion is used, so it is edited the same way as there.
function createSceneCriterionOption(
  sceneType: CriterionType,
  type: CriterionType,
  sfwMessageID?: string
) {
  const option = SceneListFilterOptions.criterionOptions.find(
    (o) => o.type === sceneType
  );
  if (!option) {
    throw new Error(`scene criterion ${sceneType} not found`);
  }
  return createSubFilterCriterionOption(
    option,
    "scene_filter",
    type,
    type,
    sfwMessageID
  );
}

const sceneCriterionOptions = [
  createSceneCriterionOption("title", "scene_title"),
  createSceneCriterionOption("path", "scene_path"),
  createSceneCriterionOption("studios", "scene_studios"),
  createSceneCriterionOption("groups", "scene_groups"),
  createSceneCriterionOption("performer_tags", "scene_performer_tags"),
  createSceneCriterionOption("performer_count", "scene_performer_count"),
  createSceneCriterionOption("performer_favorite", "scene_performer_favorite"),
  createSceneCriterionOption("performer_age", "scene_performer_age"),
  createSceneCriterionOption("rating100", "scene_rating"),
  createSceneCriterionOption("organized", "scene_organized"),
  createSceneCriterionOption("orientation", "scene_orientation"),
  createSceneCriterionOption("resolution", "scene_resolution"),
  createSceneCriterionOption("duration", "scene_duration"),
  createSceneCriterionOption("play_count", "scene_play_count"),
  createSceneCriterionOption(
    "o_counter",
    "scene_o_counter",
    "scene_o_counter_sfw"
  ),
  createSceneCriterionOption("interactive", "scene_interactive"),
];

const criterionOptions = [
  TagsCriterionOption,
  MarkersScenesCriterionOption,
  SceneTagsCriterionOption,
  PerformersCriterionOption,
  createNullDurationCriterionOption("duration"),
  createMandatoryTimestampCriterionOption("created_at"),
  createMandatoryTimestampCriterionOption("updated_at"),
  createDateCriterionOption("scene_date"),
  createMandatoryTimestampCriterionOption("scene_created_at"),
  createMandatoryTimestampCriterionOption("scene_updated_at"),
  ...sceneCriterionOptions,
];

export const SceneMarkerListFilterOptions = new ListFilterOptions(
  defaultSortBy,
  sortByOptions,
  displayModeOptions,
  criterionOptions
);
