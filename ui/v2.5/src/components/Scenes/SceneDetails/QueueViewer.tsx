import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import cx from "classnames";
import { Button, Form, Spinner } from "react-bootstrap";
import { Icon } from "src/components/Shared/Icon";
import { useIntl } from "react-intl";
import {
  faChevronDown,
  faChevronUp,
  faRandom,
  faRepeat,
  faStepBackward,
  faStepForward,
} from "@fortawesome/free-solid-svg-icons";
import { objectTitle } from "src/core/files";
import {
  QueuedScene,
  QueuedSceneMarker,
  QueuedItem,
  isCurrentQueuedItem,
  isQueuedMarker,
} from "src/models/sceneQueue";
import { markerTitle } from "src/core/markers";
import TextUtils from "src/utils/text";

export interface IPlaylistViewer {
  scenes: QueuedItem[];
  currentID?: string;
  currentMarkerSeconds?: number;
  start?: number;
  continue?: boolean;
  hasMoreScenes: boolean;
  loopQueue: boolean;
  loopScene: boolean;
  setLoopQueue: (v: boolean) => void;
  setLoopScene: (v: boolean) => void;
  setContinue: (v: boolean) => void;
  onSceneClicked: (scene: QueuedItem) => void;
  onNext: () => void;
  onPrevious: () => void;
  onRandom: () => void;
  onMoreScenes: () => void;
  onLessScenes: () => void;
}

export const QueueViewer: React.FC<IPlaylistViewer> = ({
  scenes,
  currentID,
  currentMarkerSeconds,
  start = 0,
  continue: continuePlaylist = false,
  hasMoreScenes,
  loopQueue,
  loopScene,
  setLoopQueue,
  setLoopScene,
  setContinue,
  onNext,
  onPrevious,
  onRandom,
  onSceneClicked,
  onMoreScenes,
  onLessScenes,
}) => {
  const intl = useIntl();
  const [lessLoading, setLessLoading] = useState(false);
  const [moreLoading, setMoreLoading] = useState(false);

  function isCurrent(item: QueuedItem) {
    return isCurrentQueuedItem(
      item,
      currentID ?? "",
      currentMarkerSeconds ?? 0
    );
  }

  // HACK - this isn't a great way to handle loading state. Requires a larger refactor.
  // TODO - refactor this
  // biome-ignore lint/correctness/useExhaustiveDependencies: explicitly want to set loading to false when scenes change
  useEffect(() => {
    setLessLoading(false);
    setMoreLoading(false);
  }, [scenes]);

  function handleSceneClick(
    event: React.MouseEvent<HTMLAnchorElement, MouseEvent>,
    scene: QueuedItem
  ) {
    onSceneClicked(scene);
    event.preventDefault();
  }

  function lessClicked() {
    setLessLoading(true);
    onLessScenes();
  }

  function moreClicked() {
    setMoreLoading(true);
    onMoreScenes();
  }

  function handleLoopClick() {
    if (loopQueue) {
      setLoopQueue(false);
      setLoopScene(true);
    } else if (loopScene) {
      setLoopQueue(false);
      setLoopScene(false);
    } else {
      setLoopQueue(true);
      setLoopScene(false);
    }
  }

  function renderPlaylistEntryScene(scene: QueuedScene) {
    return (
      <li className={cx("my-2", { current: isCurrent(scene) })} key={scene.id}>
        <Link
          to={`/scenes/${scene.id}`}
          onClick={(e) => handleSceneClick(e, scene)}
        >
          <div className="ml-1 d-flex align-items-center">
            <div className="thumbnail-container">
              <img
                loading="lazy"
                alt={scene.title ?? ""}
                src={scene.paths.screenshot ?? ""}
              />
            </div>
            <div className="queue-scene-details">
              <span className="queue-scene-title">{objectTitle(scene)}</span>
              <span className="queue-scene-studio">{scene?.studio?.name}</span>
              <span className="queue-scene-performers">
                {scene?.performers
                  ?.map((performer) => {
                    return performer.name;
                  })
                  .join(", ")}
              </span>
              <span className="queue-scene-date">{scene?.date}</span>
            </div>
          </div>
        </Link>
      </li>
    );
  }

  function renderPlaylistEntryMarker(marker: QueuedSceneMarker) {
    const tags = [marker.primary_tag, ...(marker.tags || [])];

    return (
      <li
        className={cx("my-2", { current: isCurrent(marker) })}
        key={marker.id}
      >
        <Link
          to={`/scenes/${marker.scene.id}?t=${marker.seconds}`}
          onClick={(e) => handleSceneClick(e, marker)}
        >
          <div className="ml-1 d-flex align-items-center">
            <div className="thumbnail-container">
              <img
                loading="lazy"
                alt={markerTitle(marker)}
                src={marker.screenshot ?? ""}
              />
            </div>
            <div className="queue-marker-details">
              <span className="queue-marker-title">
                {markerTitle(marker)}
                {" - "}
                {TextUtils.formatTimestampRange(
                  marker.seconds,
                  marker.end_seconds ?? undefined
                )}
              </span>
              <span className="queue-marker-scene">
                {objectTitle(marker.scene)}
              </span>
              <span className="queue-marker-performers">
                {marker.scene.performers
                  ?.map((performer) => performer.name)
                  .join(", ")}
              </span>
              <span className="queue-marker-tags">
                {tags.map((tag) => tag.name).join(", ")}
              </span>
            </div>
          </div>
        </Link>
      </li>
    );
  }

  return (
    <div id="queue-viewer">
      <div className="queue-controls">
        <div>
          <Form.Check
            id="continue-checkbox"
            checked={continuePlaylist}
            label={intl.formatMessage({ id: "actions.continue" })}
            onChange={() => {
              setContinue(!continuePlaylist);
            }}
          />
          <Button
            className="minimal"
            variant="secondary"
            disabled={!continuePlaylist}
            active={loopScene || loopQueue}
            title={intl.formatMessage({
              id: loopScene
                ? "actions.loop_scene"
                : loopQueue
                  ? "actions.loop_queue"
                  : "actions.loop_off",
            })}
            onClick={() => handleLoopClick()}
          >
            <Icon icon={faRepeat} />
            {loopScene && 1}
          </Button>
        </div>
        <div>
          <Button
            className="minimal"
            variant="secondary"
            disabled={scenes.length <= 1}
            onClick={() => onPrevious()}
          >
            <Icon icon={faStepBackward} />
          </Button>
          <Button
            className="minimal"
            variant="secondary"
            disabled={scenes.length <= 1}
            onClick={() => onNext()}
          >
            <Icon icon={faStepForward} />
          </Button>
          <Button
            className="minimal"
            variant="secondary"
            disabled={scenes.length <= 1}
            onClick={() => onRandom()}
          >
            <Icon icon={faRandom} />
          </Button>
        </div>
      </div>
      <div id="queue-content">
        {start > 1 ? (
          <div className="d-flex justify-content-center">
            <Button onClick={() => lessClicked()} disabled={lessLoading}>
              {!lessLoading ? (
                <Icon icon={faChevronUp} />
              ) : (
                <Spinner animation="border" role="status" />
              )}
            </Button>
          </div>
        ) : undefined}
        <ol start={start}>
          {scenes.map((item) =>
            isQueuedMarker(item)
              ? renderPlaylistEntryMarker(item)
              : renderPlaylistEntryScene(item)
          )}
        </ol>
        {hasMoreScenes ? (
          <div className="d-flex justify-content-center">
            <Button onClick={() => moreClicked()} disabled={moreLoading}>
              {!moreLoading ? (
                <Icon icon={faChevronDown} />
              ) : (
                <Spinner animation="border" role="status" />
              )}
            </Button>
          </div>
        ) : undefined}
      </div>
    </div>
  );
};

export default QueueViewer;
