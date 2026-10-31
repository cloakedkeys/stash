import * as GQL from "src/core/generated-graphql";

export interface IScraperSource {
  id: string;
  displayName: string;
  stash_box_endpoint?: string;
  scraper_id?: string;
  options?: GQL.IdentifyMetadataOptionsInput;
}

export const sceneFields = [
  "title",
  "code",
  "date",
  "production_date",
  "director",
  "details",
  "url",
  "studio",
  "performers",
  "tags",
  "stash_ids",
] as const;
export type SceneField = (typeof sceneFields)[number];

// the fields that gallery identify sets
export const galleryFields = [
  "title",
  "code",
  "date",
  "photographer",
  "details",
  "url",
  "studio",
  "performers",
  "tags",
] as const;
export type GalleryField = (typeof galleryFields)[number];

export type IdentifyType = "scene" | "gallery";
export type IdentifyField = SceneField | GalleryField;

export const multiValueSceneFields: IdentifyField[] = [
  "studio",
  "performers",
  "tags",
];

export function sceneFieldMessageID(field: IdentifyField) {
  if (field === "code") {
    return "scene_code";
  } else if (field === "studio") {
    return "studio_and_parent";
  }

  return field;
}
