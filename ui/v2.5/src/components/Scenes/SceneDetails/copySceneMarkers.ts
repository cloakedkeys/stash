import type {
  SceneDataFragment,
  SceneMarkerCreateMutationVariables,
} from "src/core/generated-graphql";

export type SplitSceneMarker = Pick<
  SceneDataFragment["scene_markers"][number],
  "title" | "seconds" | "end_seconds" | "primary_tag" | "tags"
>;

export async function copySceneMarkers(
  markers: readonly SplitSceneMarker[],
  sceneID: string,
  create: (input: SceneMarkerCreateMutationVariables) => Promise<boolean>,
  enabled: boolean
): Promise<number> {
  if (!enabled) return 0;

  let failed = 0;
  for (const marker of markers) {
    try {
      const created = await create({
        scene_id: sceneID,
        title: marker.title,
        seconds: marker.seconds,
        end_seconds: marker.end_seconds ?? null,
        primary_tag_id: marker.primary_tag.id,
        tag_ids: marker.tags.map((tag) => tag.id),
      });
      if (!created) failed++;
    } catch {
      failed++;
    }
  }
  return failed;
}
