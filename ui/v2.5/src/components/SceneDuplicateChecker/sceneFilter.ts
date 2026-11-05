import * as GQL from "src/core/generated-graphql";
import { ListFilterModel } from "src/models/list-filter/filter";

// The duplicate checker keeps its scene filter in the page address, in the
// same "c" parameters the scene list uses, so a reload or a shared link keeps
// it. Only the criteria are used: the duplicate search has no search text,
// sort or paging of its own.

export function sceneFilterFromQuery(
  search: string,
  config?: GQL.ConfigDataFragment
) {
  const filter = new ListFilterModel(GQL.FilterMode.Scenes, config);
  const query = new URLSearchParams(search);
  filter.configureFromDecodedParams(
    ListFilterModel.decodeParams({ c: query.getAll("c") })
  );
  return filter;
}

// Returns a copy of query with the filter's criteria in place of any old ones.
export function setSceneFilterQuery(
  query: URLSearchParams,
  filter: ListFilterModel
) {
  const ret = new URLSearchParams(query);
  ret.delete("c");
  for (const c of filter.getEncodedParams().c ?? []) {
    // getEncodedParams escapes for a hand-built address; URLSearchParams
    // escapes again, so undo the first one
    ret.append("c", decodeURIComponent(c));
  }
  return ret;
}

export function makeSceneFilter(
  filter: ListFilterModel
): GQL.SceneFilterType | undefined {
  if (filter.criteria.length === 0) {
    return undefined;
  }
  return filter.makeFilter();
}
