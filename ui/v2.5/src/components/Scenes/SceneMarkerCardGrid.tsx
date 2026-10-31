import React from "react";
import * as GQL from "src/core/generated-graphql";
import { SceneMarkerCard } from "./SceneMarkerCard";
import {
  useCardWidth,
  useContainerDimensions,
} from "../Shared/GridCard/GridCard";
import { PatchComponent } from "src/patch";
import { SceneQueue } from "src/models/sceneQueue";

interface ISceneMarkerCardGrid {
  markers: GQL.SceneMarkerDataFragment[];
  queue?: SceneQueue;
  selectedIds: Set<string>;
  zoomIndex: number;
  onSelectChange: (id: string, selected: boolean, shiftKey: boolean) => void;
}

const zoomWidths = [240, 340, 480, 640];

export const SceneMarkerCardGrid: React.FC<ISceneMarkerCardGrid> =
  PatchComponent(
    "SceneMarkerCardGrid",
    ({ markers, queue, selectedIds, zoomIndex, onSelectChange }) => {
      const [componentRef, { width: containerWidth }] =
        useContainerDimensions();
      const cardWidth = useCardWidth(containerWidth, zoomIndex, zoomWidths);

      return (
        <div className="row justify-content-center" ref={componentRef}>
          {markers.map((marker, index) => (
            <SceneMarkerCard
              key={marker.id}
              cardWidth={cardWidth}
              marker={marker}
              queue={queue}
              index={index}
              zoomIndex={zoomIndex}
              selecting={selectedIds.size > 0}
              selected={selectedIds.has(marker.id)}
              onSelectedChanged={(selected: boolean, shiftKey: boolean) =>
                onSelectChange(marker.id, selected, shiftKey)
              }
            />
          ))}
        </div>
      );
    }
  );
