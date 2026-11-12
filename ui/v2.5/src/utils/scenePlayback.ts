import type { SceneDataFragment } from "src/core/generated-graphql";

type SceneRange = Pick<SceneDataFragment, "start_time" | "end_time">;

export function isPartScene(scene: SceneRange) {
  return scene.start_time != null || scene.end_time != null;
}

export function scenePlaybackDuration(scene: SceneRange, fileDuration: number) {
  return isPartScene(scene)
    ? Math.max(0, (scene.end_time ?? fileDuration) - (scene.start_time ?? 0))
    : fileDuration;
}

// Generated media and the range editor still use the source file's timeline.
export function sceneTimeToFileTime(scene: SceneRange, time: number) {
  return (scene.start_time ?? 0) + time;
}
