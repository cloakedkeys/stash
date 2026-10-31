import {
  FilterMode,
  Scene,
  SceneMarker,
  Tag,
} from "src/core/generated-graphql";
import { ListFilterModel } from "./list-filter/filter";
import { INamedObject } from "src/utils/navigation";
import { IFile } from "src/core/files";

export type QueuedScene = Pick<Scene, "id" | "title" | "date" | "paths"> & {
  files: IFile[];
  performers?: INamedObject[] | null;
  studio?: INamedObject | null;
};

export type QueuedSceneMarker = Pick<
  SceneMarker,
  "id" | "seconds" | "title" | "screenshot" | "end_seconds"
> & {
  tags?: INamedObject[] | null;
  primary_tag: Pick<Tag, "id" | "name">;
  scene: Pick<Scene, "id" | "title"> & {
    files: IFile[];
    performers?: INamedObject[] | null;
  };
};

export type QueuedItem = QueuedScene | QueuedSceneMarker;

export function isQueuedMarker(item: QueuedItem): item is QueuedSceneMarker {
  return "seconds" in item;
}

// the id of the scene that plays a queued scene or marker
export function queuedItemSceneID(item: QueuedItem) {
  return isQueuedMarker(item) ? item.scene.id : item.id;
}

// whether a queued item is the one being shown. Markers are matched on their
// scene and their start time (whole seconds), since the scene page URL only
// carries the scene id and the start time.
export function isCurrentQueuedItem(
  item: QueuedItem,
  sceneID: string,
  timestamp: number
) {
  if (isQueuedMarker(item)) {
    return (
      item.scene.id === sceneID &&
      Math.trunc(item.seconds) === Math.trunc(timestamp)
    );
  }

  return item.id === sceneID;
}

export interface IPlaySceneOptions {
  sceneIndex?: number;
  newPage?: number;
  autoPlay?: boolean;
  continue?: boolean;
  start?: number;
  end?: number | null;
}

export class SceneQueue {
  public query?: ListFilterModel;
  public sceneIDs?: number[];
  public sceneMarkerIDs?: number[];
  private originalQueryPage?: number;
  private originalQueryPageSize?: number;

  public static fromListFilterModel(filter: ListFilterModel) {
    const ret = new SceneQueue();

    const filterCopy = filter.clone();
    filterCopy.itemsPerPage = 40;

    ret.originalQueryPage = filter.currentPage;
    ret.originalQueryPageSize = filter.itemsPerPage;

    ret.query = filterCopy;
    return ret;
  }

  public static fromSceneIDList(sceneIDs: string[]) {
    const ret = new SceneQueue();
    ret.sceneIDs = sceneIDs.map((v) => Number(v));
    return ret;
  }

  public static fromSceneMarkerIDList(sceneMarkerIDs: string[]) {
    const ret = new SceneQueue();
    ret.sceneMarkerIDs = sceneMarkerIDs.map((v) => Number(v));
    return ret;
  }

  private makeQueryParameters(sceneIndex?: number, page?: number) {
    const ret: string[] = [];

    if (this.query) {
      const queryParams = this.query.getEncodedParams();

      if (queryParams.sortby) {
        ret.push(`qsort=${queryParams.sortby}`);
      }
      if (queryParams.sortdir) {
        ret.push(`qsortd=${queryParams.sortdir}`);
      }
      if (queryParams.q) {
        ret.push(`qfq=${queryParams.q}`);
      }
      for (const c of queryParams.c ?? []) {
        ret.push(`qfc=${c}`);
      }

      let qfp = queryParams.p ?? "1";
      if (page !== undefined) {
        qfp = String(page);
      } else if (
        sceneIndex !== undefined &&
        this.originalQueryPage !== undefined &&
        this.originalQueryPageSize !== undefined
      ) {
        // adjust page to be correct for the index
        const filterIndex =
          sceneIndex +
          (this.originalQueryPage - 1) * this.originalQueryPageSize;
        const newPage = Math.floor(filterIndex / this.query.itemsPerPage) + 1;
        qfp = String(newPage);
      }
      ret.push(`qfp=${qfp}`);

      if (this.query.mode === FilterMode.SceneMarkers) {
        ret.push("qfm=scene_marker");
      }
    } else if (this.sceneIDs && this.sceneIDs.length > 0) {
      for (const id of this.sceneIDs) {
        ret.push(`qs=${id}`);
      }
    } else if (this.sceneMarkerIDs && this.sceneMarkerIDs.length > 0) {
      for (const id of this.sceneMarkerIDs) {
        ret.push(`qm=${id}`);
      }
    }

    return ret.join("&");
  }

  public static fromQueryParameters(params: URLSearchParams) {
    const ret = new SceneQueue();

    if (params.has("qfp")) {
      const translated = {
        sortby: params.get("qsort"),
        sortdir: params.get("qsortd"),
        q: params.get("qfq"),
        p: params.get("qfp"),
        c: params.getAll("qfc"),
      };

      const filterMode =
        params.get("qfm") === "scene_marker"
          ? FilterMode.SceneMarkers
          : FilterMode.Scenes;

      const decoded = ListFilterModel.decodeParams(translated);
      const query = new ListFilterModel(filterMode);
      query.configureFromDecodedParams(decoded);
      ret.query = query;
    } else if (params.has("qs")) {
      // must be scene list
      ret.sceneIDs = params.getAll("qs").map((v) => Number(v));
    } else if (params.has("qm")) {
      // must be marker list
      ret.sceneMarkerIDs = params.getAll("qm").map((v) => Number(v));
    }

    return ret;
  }

  public makeLink(sceneID: string, options: IPlaySceneOptions) {
    const params = [
      this.makeQueryParameters(options.sceneIndex, options.newPage),
    ];
    if (options.autoPlay) {
      params.push("autoplay=true");
    }
    if (options.continue !== undefined) {
      params.push("continue=" + options.continue);
    }
    if (options.start !== undefined) {
      if (options.end) {
        params.push("t=" + options.start + "," + options.end);
      } else {
        params.push("t=" + options.start);
      }
    }
    return `/scenes/${sceneID}${params.length ? "?" + params.join("&") : ""}`;
  }
}

export default SceneQueue;
