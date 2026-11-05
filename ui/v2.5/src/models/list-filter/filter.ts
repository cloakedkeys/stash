import {
  ConfigDataFragment,
  CriterionModifier,
  FilterMode,
  FindFilterType,
  SavedFilterDataFragment,
  SortDirectionEnum,
} from "src/core/generated-graphql";
import {
  Criterion,
  ModifierCriterion,
  UnsupportedCriterionOption,
} from "./criteria/criterion";
import { getFilterOptions } from "./factory";
import { CriterionType, DisplayMode, SavedUIOptions } from "./types";
import { ListFilterOptions } from "./filter-options";
import { CustomFieldsCriterion } from "./criteria/custom-fields";

interface IDecodedParams {
  perPage?: number;
  sortby?: string;
  sortdir?: string;
  disp?: DisplayMode;
  q?: string;
  p?: number;
  z?: number;
  c?: string[];
}

interface IEncodedParams {
  perPage?: string | null;
  sortby?: string | null;
  sortdir?: string | null;
  disp?: string | null;
  q?: string | null;
  p?: string | null;
  z?: string | null;
  c?: string[];
}

const DEFAULT_PARAMS = {
  sortDirection: SortDirectionEnum.Asc,
  displayMode: DisplayMode.Grid,
  currentPage: 1,
  itemsPerPage: 40,
};

// TODO: handle customCriteria
// The most items a list shows on one page. Bigger pages are slow, especially
// on a phone, so a web address or saved filter asking for more gets this many
// (#1954). 0 or less is left alone: it has its own meaning (everything).
export const MAX_PAGE_SIZE = 255;

export function limitPageSize(pageSize: number) {
  return pageSize > MAX_PAGE_SIZE ? MAX_PAGE_SIZE : pageSize;
}

// Object filter types have AND, OR and NOT sub-filters of their own type,
// and an ALL_OF list of them, except markers (see supportsSubFilters).
const AND_KEY = "AND";
const OR_KEY = "OR";
const NOT_KEY = "NOT";
const ALL_OF_KEY = "ALL_OF";

// In the web address a group is one c= value: {"type":"_any","c":[...]}, with
// a criterion in the usual form for each of its conditions.
const URL_GROUP_TYPES: Record<string, GroupKind> = {
  _any: "any",
  _none: "none",
};
// in the web address, set on a condition that also matches empty values
const URL_OR_EMPTY_KEY = "or_empty";

// The list modes whose server filter type has no AND, OR or NOT
// (SceneMarkerFilterType). A test checks this against the GraphQL schema.
const MODES_WITHOUT_SUB_FILTERS = new Set<FilterMode>([
  FilterMode.SceneMarkers,
]);

// Whether the server filter type of the mode has AND, OR and NOT
// sub-filters, which repeated conditions need.
export function supportsSubFilters(mode: FilterMode) {
  return !MODES_WITHOUT_SUB_FILTERS.has(mode);
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

// Any of: at least one condition must match. None of: no condition may match.
export type GroupKind = "any" | "none";

export interface IFilterGroup {
  // the name its criteria have in filterGroup
  name: string;
  kind: GroupKind;
  // 1 for the first group of its kind, and so on
  number: number;
  criteria: Criterion[];
}

export function groupName(kind: GroupKind, id: number) {
  return `${kind}:${id}`;
}

export function parseGroupName(
  name?: string
): { kind: GroupKind; id: number } | undefined {
  const match = name?.match(/^(any|none):(\d+)$/);
  if (!match) return undefined;
  return { kind: match[1] as GroupKind, id: Number.parseInt(match[2], 10) };
}

// the Any of / None of groups of criteria, in the order they first appear
export function filterGroups(criteria: Criterion[]): IFilterGroup[] {
  const ret: IFilterGroup[] = [];
  for (const c of criteria) {
    const parsed = parseGroupName(c.filterGroup);
    if (!parsed || ret.some((g) => g.name === c.filterGroup)) continue;
    ret.push({
      name: c.filterGroup!,
      kind: parsed.kind,
      number: ret.filter((g) => g.kind === parsed.kind).length + 1,
      criteria: criteria.filter((cc) => cc.filterGroup === c.filterGroup),
    });
  }
  return ret;
}

// Whether the condition can be set to also match items where the value is
// empty (#1170): it has an "is empty" form and doesn't use it already.
export function canMatchEmpty(c: Criterion) {
  return (
    c instanceof ModifierCriterion &&
    (c.modifierCriterionOption().modifierOptions ?? []).includes(
      CriterionModifier.IsNull
    ) &&
    c.modifier !== CriterionModifier.IsNull &&
    c.modifier !== CriterionModifier.NotNull
  );
}

// whether the condition is in an Any of / None of group
export function inGroup(c: Criterion) {
  return parseGroupName(c.filterGroup) !== undefined;
}

function matchesEmptyToo(c: Criterion) {
  return !!c.orEmpty && canMatchEmpty(c);
}

function isEmptyCriterionOf(empty: Criterion, c: Criterion) {
  return (
    empty instanceof ModifierCriterion &&
    empty.modifier === CriterionModifier.IsNull &&
    empty.criterionOption.type === c.criterionOption.type &&
    canMatchEmpty(c)
  );
}

type ApplyCriterion = (c: Criterion, output: Record<string, unknown>) => void;

// Builds a filter object from criteria, each writing its own keys with apply.
// A criterion whose keys are already used, such as a second condition of the
// same kind, goes into the AND sub-filter instead, one level deeper for each
// repeat: {is_missing: A, AND: {is_missing: B, AND: {is_missing: C}}}. The
// conditions are all ANDed, as they are at the top level. Without repeats the
// result is exactly what writing every criterion into one object gives.
// Keys in mergeKeys hold objects that several criteria can share, as long as
// the keys inside them differ (criteria on a related object, #1548).
// Without useAnd (the filter type has no AND), a repeat replaces the earlier
// condition, as all repeats did before they were allowed.
function combineCriteria(
  criteria: Criterion[],
  apply: ApplyCriterion,
  mergeKeys?: Set<string>,
  useAnd = true
) {
  const canMerge = (k: string, a: unknown, b: unknown) =>
    !!mergeKeys?.has(k) &&
    isPlainObject(a) &&
    isPlainObject(b) &&
    !Object.keys(a).some((kk) => kk in b);

  const output: Record<string, unknown> = {};
  for (const c of criteria) {
    const own: Record<string, unknown> = {};
    apply(c, own);

    let target = output;
    while (
      useAnd &&
      Object.keys(own).some(
        (k) => k in target && !canMerge(k, own[k], target[k])
      )
    ) {
      if (!isPlainObject(target[AND_KEY])) {
        target[AND_KEY] = {};
      }
      target = target[AND_KEY] as Record<string, unknown>;
    }
    for (const [k, v] of Object.entries(own)) {
      target[k] =
        mergeKeys?.has(k) && isPlainObject(target[k]) && isPlainObject(v)
          ? { ...target[k], ...v }
          : v;
    }
  }
  return output;
}
export class ListFilterModel {
  public readonly mode: FilterMode;
  public readonly options: ListFilterOptions;
  private config?: ConfigDataFragment;
  public searchTerm: string = "";
  public currentPage = DEFAULT_PARAMS.currentPage;
  public itemsPerPage = DEFAULT_PARAMS.itemsPerPage;
  public sortDirection: SortDirectionEnum = DEFAULT_PARAMS.sortDirection;
  public sortBy?: string;
  public displayMode: DisplayMode = DEFAULT_PARAMS.displayMode;
  public zoomIndex: number = 1;
  public criteria: Array<Criterion> = [];
  public randomSeed = -1;
  private defaultZoomIndex: number = 1;

  public constructor(
    mode: FilterMode,
    config?: ConfigDataFragment,
    options?: {
      defaultZoomIndex?: number;
      defaultSortBy?: string;
      defaultSortDir?: SortDirectionEnum;
    }
  ) {
    this.mode = mode;
    this.config = config;
    this.options = getFilterOptions(mode);
    const { defaultSortBy, displayModeOptions } = this.options;

    if (options?.defaultSortBy) {
      this.sortBy = options.defaultSortBy;
      if (options.defaultSortDir) {
        this.sortDirection = options.defaultSortDir;
      }
    } else {
      this.sortBy = defaultSortBy;
      if (this.sortBy === "date") {
        this.sortDirection = SortDirectionEnum.Desc;
      }
    }
    this.displayMode = displayModeOptions[0];
    if (options?.defaultZoomIndex !== undefined) {
      this.defaultZoomIndex = options.defaultZoomIndex;
      this.zoomIndex = options.defaultZoomIndex;
    }
  }

  public clone() {
    const ret = Object.assign(
      new ListFilterModel(this.mode, this.config),
      this
    );
    ret.criteria = this.criteria.map((c) => c.clone());
    return ret;
  }

  public empty() {
    return new ListFilterModel(this.mode, this.config, {
      defaultZoomIndex: this.defaultZoomIndex,
    });
  }

  // returns a clone of the filter for metadata fetching
  // this removes the sort, page size and page number and zoom index
  public metadataInfo() {
    const clone = this.clone();
    clone.sortBy = undefined;
    clone.randomSeed = -1;
    clone.currentPage = 1;
    clone.sortDirection = DEFAULT_PARAMS.sortDirection;
    clone.itemsPerPage = 0;
    clone.zoomIndex = 1;
    clone.displayMode = DEFAULT_PARAMS.displayMode;
    return clone;
  }

  // returns the number of filters applied
  public count() {
    // don't include search term
    return this.criteria.length;
  }

  public configureFromDecodedParams(params: IDecodedParams) {
    if (params.perPage !== undefined) {
      this.itemsPerPage = limitPageSize(params.perPage);
    }
    if (params.sortby !== undefined) {
      this.sortBy = params.sortby;

      // parse the random seed if provided
      const match = this.sortBy.match(/^random_(\d+)$/);
      if (match) {
        this.sortBy = "random";
        this.randomSeed = Number.parseInt(match[1], 10);
      }
    }
    if (params.sortdir !== undefined) {
      this.sortDirection =
        params.sortdir === "desc"
          ? SortDirectionEnum.Desc
          : SortDirectionEnum.Asc;
    } else {
      // #3193 - sortdir undefined means asc
      // #3559 - unless sortby is date, then desc
      this.sortDirection =
        params.sortby === "date"
          ? SortDirectionEnum.Desc
          : SortDirectionEnum.Asc;
    }
    if (params.disp !== undefined) {
      this.displayMode = params.disp;
    }
    if (params.q !== undefined) {
      this.searchTerm = params.q;
    }
    this.currentPage = params.p ?? 1;
    if (params.z !== undefined) {
      this.zoomIndex = params.z;
    }

    this.criteria = [];
    if (params.c !== undefined) {
      for (const jsonString of params.c) {
        try {
          const decoded = JSON.parse(jsonString);
          const groupKind = URL_GROUP_TYPES[decoded?.type];
          if (groupKind && Array.isArray(decoded.c)) {
            const name = this.newGroupName(groupKind);
            for (const item of decoded.c) {
              const criterion = this.criterionFromDecodedParams(item);
              criterion.filterGroup = name;
              this.criteria.push(criterion);
            }
            continue;
          }

          this.criteria.push(this.criterionFromDecodedParams(decoded));
        } catch (err) {
          console.error("Failed to parse encoded criterion:", err);
        }
      }
    }
  }

  private criterionFromDecodedParams(decoded: Record<string, unknown>) {
    const {
      type: criterionType,
      [URL_OR_EMPTY_KEY]: orEmpty,
      ...savedCriterion
    } = decoded;

    const criterion = this.makeCriterion(criterionType as CriterionType);
    criterion.fromDecodedParams(savedCriterion);
    if (orEmpty === true) {
      criterion.orEmpty = true;
    }
    return criterion;
  }

  private static criterionQueryParams(c: Criterion) {
    const ret = c.toQueryParams();
    if (matchesEmptyToo(c)) {
      ret[URL_OR_EMPTY_KEY] = true;
    }
    return ret;
  }

  // Does not decode any URL-encoding, only type conversions
  public static decodeParams(params: IEncodedParams): IDecodedParams {
    const ret: IDecodedParams = {};

    if (params.perPage) {
      ret.perPage = Number.parseInt(params.perPage, 10);
    }
    if (params.sortby) {
      ret.sortby = params.sortby;
    }
    if (params.sortdir) {
      ret.sortdir = params.sortdir;
    }
    if (params.disp) {
      ret.disp = Number.parseInt(params.disp, 10);
    }
    if (params.q) {
      ret.q = params.q;
    }
    if (params.p) {
      ret.p = Number.parseInt(params.p, 10);
    }
    if (params.z) {
      const zoomIndex = Number.parseInt(params.z, 10);
      if (zoomIndex >= 0) {
        ret.z = zoomIndex;
      }
    }

    if (params.c && params.c.length !== 0) {
      ret.c = params.c.map((jsonString) =>
        ListFilterModel.translateJSON(jsonString, true)
      );
    }

    return ret;
  }

  private static translateJSON(jsonString: string, decoding: boolean) {
    let inString = false;
    let isEscaped = false;
    return [...jsonString]
      .map((c) => {
        if (isEscaped) {
          // this character has been escaped, skip
          isEscaped = false;
          return c;
        }

        switch (c) {
          case "\\":
            // escape the next character if in a string
            if (inString) {
              isEscaped = true;
            }
            break;
          case '"':
            // unescaped quote, toggle inString
            inString = !inString;
            break;
          case "(":
            // decode only: restore ( to { if not in a string
            if (decoding && !inString) {
              return "{";
            }
            break;
          case ")":
            // decode only: restore ) to } if not in a string
            if (decoding && !inString) {
              return "}";
            }
            break;
          case "{":
            // encode only: replace { with ( if not in a string
            if (!decoding && !inString) {
              return "(";
            }
            break;
          case "}":
            // encode only: replace } with ) if not in a string
            if (!decoding && !inString) {
              return ")";
            }
            break;
        }

        return c;
      })
      .join("");
  }

  public configureFromQueryString(queryString: string) {
    const query = new URLSearchParams(queryString);
    const params = {
      perPage: query.get("perPage"),
      sortby: query.get("sortby"),
      sortdir: query.get("sortdir"),
      disp: query.get("disp"),
      q: query.get("q"),
      p: query.get("p"),
      z: query.get("z"),
      c: query.getAll("c"),
    };
    const decoded = ListFilterModel.decodeParams(params);
    this.configureFromDecodedParams(decoded);
  }

  public configureFromSavedFilter(savedFilter: SavedFilterDataFragment) {
    const {
      find_filter: findFilter,
      object_filter: objectFilter,
      ui_options: uiOptions,
    } = savedFilter;

    this.itemsPerPage = limitPageSize(
      findFilter?.per_page ?? this.itemsPerPage
    );
    this.sortBy = findFilter?.sort ?? this.sortBy;
    // parse the random seed if provided
    const match = this.sortBy?.match(/^random_(\d+)$/);
    if (match) {
      this.sortBy = "random";
      this.randomSeed = Number.parseInt(match[1], 10);
    }
    this.sortDirection = findFilter?.direction ?? this.sortDirection;
    this.searchTerm = findFilter?.q ?? this.searchTerm;

    this.displayMode = uiOptions?.display_mode ?? this.displayMode;
    this.zoomIndex = uiOptions?.zoom_index ?? this.zoomIndex;

    this.currentPage = 1;

    this.criteria = [];
    if (objectFilter) {
      this.addSavedCriteria(objectFilter);
    }
  }

  // Adds the criteria of a saved object filter. A repeated condition is saved
  // in an AND sub-filter of the same form (see combineCriteria); its criteria
  // are added where the sub-filter is, so they keep their order.
  private addSavedCriteria(objectFilter: Record<string, unknown>) {
    for (const [k, v] of Object.entries(objectFilter)) {
      if (k === AND_KEY && isPlainObject(v)) {
        this.addSavedCriteria(v);
        continue;
      }
      if (k === ALL_OF_KEY && Array.isArray(v)) {
        for (const item of v) {
          this.addSavedGroup(item);
        }
        continue;
      }
      const criterion = this.makeCriterion(k as CriterionType);
      criterion.setFromSavedCriterion(v);
      this.criteria.push(criterion);
    }
  }

  // Adds an ALL_OF item of a saved filter, in the form makeObjectFilter
  // writes: {a, OR: {b}} is an Any of group, {NOT: {a, OR: {b}}} a None of
  // group.
  private addSavedGroup(item: unknown) {
    if (!isPlainObject(item)) return;

    let kind: GroupKind = "any";
    let node: unknown = item;
    const keys = Object.keys(item);
    if (keys.length === 1 && keys[0] === NOT_KEY) {
      kind = "none";
      node = item[NOT_KEY];
    }

    const criteria: Criterion[] = [];
    for (; isPlainObject(node); node = node[OR_KEY]) {
      for (const [k, v] of Object.entries(node)) {
        if (k === OR_KEY) continue;
        const criterion = this.makeCriterion(k as CriterionType);
        criterion.setFromSavedCriterion(v);
        criteria.push(criterion);
      }
    }

    this.addGroup(kind, criteria);
  }

  // Adds criteria as a new group. A condition followed by its "is empty"
  // form becomes one condition that also matches empty values. An Any of
  // group left with just that one condition is not needed as a group.
  private addGroup(kind: GroupKind, criteria: Criterion[]) {
    const merged: Criterion[] = [];
    for (const c of criteria) {
      const prev = merged[merged.length - 1];
      if (prev && !prev.orEmpty && isEmptyCriterionOf(c, prev)) {
        prev.orEmpty = true;
        continue;
      }
      merged.push(c);
    }

    if (merged.length === 0) return;

    if (kind === "any" && merged.length === 1 && merged[0].orEmpty) {
      this.criteria.push(merged[0]);
      return;
    }

    const name = this.newGroupName(kind);
    for (const c of merged) {
      c.filterGroup = name;
      this.criteria.push(c);
    }
  }

  private setRandomSeed() {
    if (this.sortBy === "random") {
      // #321 - set the random seed if it is not set
      if (this.randomSeed === -1) {
        // generate 8-digit seed
        this.randomSeed = Math.floor(Math.random() * 10 ** 8);
      }
    } else {
      this.randomSeed = -1;
    }
  }

  private getSortBy(): string | undefined {
    this.setRandomSeed();

    if (this.sortBy === "random") {
      return `random_${this.randomSeed.toString()}`;
    }

    return this.sortBy;
  }

  // Returns query parameters with necessary parts URL-encoded
  public getEncodedParams(): IEncodedParams {
    // a group is one value, where its first condition is
    const values: Record<string, unknown>[] = [];
    const seen = new Set<string>();
    for (const c of this.criteria) {
      const group = parseGroupName(c.filterGroup);
      if (!group) {
        values.push(ListFilterModel.criterionQueryParams(c));
        continue;
      }
      if (seen.has(c.filterGroup!)) continue;
      seen.add(c.filterGroup!);
      values.push({
        type: group.kind === "none" ? "_none" : "_any",
        c: this.criteria
          .filter((cc) => cc.filterGroup === c.filterGroup)
          .map((cc) => ListFilterModel.criterionQueryParams(cc)),
      });
    }

    const encodedCriteria: string[] = values.map((queryParams) => {
      let str = ListFilterModel.translateJSON(
        JSON.stringify(queryParams),
        false
      );

      // URL-encode other characters
      str = encodeURI(str);

      // only the reserved characters ?#&;=+ need to be URL-encoded
      // as they have special meaning in query strings
      str = str.replaceAll("?", encodeURIComponent("?"));
      str = str.replaceAll("#", encodeURIComponent("#"));
      str = str.replaceAll("&", encodeURIComponent("&"));
      str = str.replaceAll(";", encodeURIComponent(";"));
      str = str.replaceAll("=", encodeURIComponent("="));
      str = str.replaceAll("+", encodeURIComponent("+"));

      return str;
    });

    return {
      perPage:
        this.itemsPerPage !== DEFAULT_PARAMS.itemsPerPage
          ? String(this.itemsPerPage)
          : undefined,
      sortby: this.getSortBy(),
      sortdir:
        this.sortBy === "date"
          ? this.sortDirection === SortDirectionEnum.Asc
            ? "asc"
            : undefined
          : this.sortDirection === SortDirectionEnum.Desc
            ? "desc"
            : undefined,
      disp:
        this.displayMode !== DEFAULT_PARAMS.displayMode
          ? String(this.displayMode)
          : undefined,
      q: this.searchTerm ? encodeURIComponent(this.searchTerm) : undefined,
      p:
        this.currentPage !== DEFAULT_PARAMS.currentPage
          ? String(this.currentPage)
          : undefined,
      z:
        this.zoomIndex !== this.defaultZoomIndex
          ? String(this.zoomIndex)
          : undefined,
      c: encodedCriteria,
    };
  }

  public makeQueryParameters(): string {
    const query: string[] = [];
    const params = this.getEncodedParams();

    if (params.q) {
      query.push(`q=${params.q}`);
    }
    if (params.c) {
      for (const c of params.c) {
        query.push(`c=${c}`);
      }
    }
    if (params.sortby) {
      query.push(`sortby=${params.sortby}`);
    }
    if (params.sortdir) {
      query.push(`sortdir=${params.sortdir}`);
    }
    if (params.perPage) {
      query.push(`perPage=${params.perPage}`);
    }
    if (params.disp) {
      query.push(`disp=${params.disp}`);
    }
    if (params.z) {
      query.push(`z=${params.z}`);
    }
    if (params.p) {
      query.push(`p=${params.p}`);
    }

    return query.join("&");
  }

  public makeCriterion(type: CriterionType) {
    const { criterionOptions } = getFilterOptions(this.mode);

    const option = criterionOptions.find((o) => o.type === type);

    if (!option) {
      return new UnsupportedCriterionOption(type).makeCriterion(this.config);
    }

    return option.makeCriterion(this.config);
  }

  public makeFindFilter(): FindFilterType {
    return {
      q: this.searchTerm,
      page: this.currentPage,
      per_page: this.itemsPerPage,
      sort: this.getSortBy(),
      direction: this.sortDirection,
    };
  }

  // A list whose server filter type has no AND (markers) can't send repeated
  // conditions. Such a list doesn't offer repeats, but one can still come
  // from an edited web address or a saved filter; then the last condition of
  // the kind is sent, as before repeats were allowed.
  public makeFilter() {
    // criteria on a related object go inside its filter, under the related
    // object's own name for the criterion. Several of them share that filter;
    // a repeat of one goes into AND like any other.
    const subFilterKeys = new Set<string>();
    for (const c of this.criteria) {
      const { subFilter } = c.criterionOption;
      if (subFilter) subFilterKeys.add(subFilter.key);
    }

    return this.makeObjectFilter(
      (c, output) => {
        const { type, subFilter } = c.criterionOption;
        if (!subFilter) {
          c.applyToCriterionInput(output);
          return;
        }

        const subOutput: Record<string, unknown> = {};
        c.applyToCriterionInput(subOutput);
        if (type in subOutput) {
          subOutput[subFilter.field] = subOutput[type];
          delete subOutput[type];
        }
        output[subFilter.key] = subOutput;
      },
      subFilterKeys,
      supportsSubFilters(this.mode)
    );
  }

  // Saved filters are read only by the UI, so they keep every condition,
  // in the AND form, even for lists that can't send it.
  // TODO - this needs to just use makeFilter, but it needs a migration
  public makeSavedFilter() {
    return this.makeObjectFilter((c, output) =>
      c.applyToSavedCriterion(output)
    );
  }

  // Builds the filter object. Conditions outside groups are written as
  // before (see combineCriteria). Each Any of / None of group, and each
  // condition outside groups that also matches empty values, is one item of
  // ALL_OF, so that several groups can be used together (#1206):
  //   Any of [a, b]:  {a, OR: {b}}
  //   None of [a, b]: {NOT: {a, OR: {b}}}
  // Without groups the result is exactly what it was before.
  // Without useAnd (the filter type has no AND, OR, NOT or ALL_OF) groups
  // and "or empty" can't be sent. The dialog doesn't offer them then; if they
  // come from an edited web address, conditions in groups are left out (a
  // None of group sent as a plain condition would mean the opposite), the
  // others are sent without "or empty", and a repeat replaces the earlier
  // one, as before.
  private makeObjectFilter(
    apply: ApplyCriterion,
    mergeKeys?: Set<string>,
    useAnd = true
  ) {
    if (!useAnd) {
      return combineCriteria(
        this.criteria.filter((c) => !inGroup(c)),
        apply,
        mergeKeys,
        false
      );
    }

    const output = combineCriteria(
      this.criteria.filter((c) => !inGroup(c) && !matchesEmptyToo(c)),
      apply,
      mergeKeys
    );

    const items: Record<string, unknown>[] = [];
    const seen = new Set<string>();
    for (const c of this.criteria) {
      if (!inGroup(c)) {
        if (matchesEmptyToo(c)) {
          items.push(this.anyOf([c], apply, mergeKeys));
        }
        continue;
      }

      if (seen.has(c.filterGroup!)) continue;
      seen.add(c.filterGroup!);

      const group = this.criteria.filter(
        (cc) => cc.filterGroup === c.filterGroup
      );
      const anyOf = this.anyOf(group, apply, mergeKeys);
      items.push(
        parseGroupName(c.filterGroup)?.kind === "none"
          ? { [NOT_KEY]: anyOf }
          : anyOf
      );
    }

    if (items.length > 0) {
      output[ALL_OF_KEY] = items;
    }
    return output;
  }

  // {a, OR: {b, OR: {c}}}; a condition that also matches empty values is
  // followed by the same condition with "is empty"
  private anyOf(
    criteria: Criterion[],
    apply: ApplyCriterion,
    mergeKeys?: Set<string>
  ) {
    const all = criteria.flatMap((c) =>
      matchesEmptyToo(c) ? [c, this.makeEmptyCriterion(c)] : [c]
    );

    let ret: Record<string, unknown> | undefined;
    for (const c of all.reverse()) {
      const item = combineCriteria([c], apply, mergeKeys);
      if (ret) item[OR_KEY] = ret;
      ret = item;
    }
    return ret ?? {};
  }

  // the "is empty" condition of the same kind as c
  private makeEmptyCriterion(c: Criterion) {
    const ret = this.makeCriterion(c.criterionOption.type);
    if (ret instanceof ModifierCriterion) {
      ret.modifier = CriterionModifier.IsNull;
    }
    return ret;
  }

  // Any of / None of groups, in the order they first appear in criteria
  public groups(): IFilterGroup[] {
    return filterGroups(this.criteria);
  }

  // a name for a new group of the given kind
  public newGroupName(kind: GroupKind) {
    let id = 1;
    for (const c of this.criteria) {
      const parsed = parseGroupName(c.filterGroup);
      if (parsed && parsed.id >= id) id = parsed.id + 1;
    }
    return groupName(kind, id);
  }

  public makeSavedUIOptions(): SavedUIOptions {
    return {
      display_mode: this.displayMode,
      zoom_index: this.zoomIndex,
    };
  }

  // The conditions of the given kind outside Any of / None of groups. These
  // are what the sidebar filters show and edit.
  public criteriaFor(type: CriterionType) {
    return this.criteria.filter(
      (c) => c.criterionOption.type === type && !inGroup(c)
    );
  }

  // Replaces the first criterion of the given kind, the one the sidebar
  // filters show and edit, with newCriteria (none removes it). Other criteria
  // of that kind are kept. With no others, newCriteria go at the end, as they
  // always have.
  public replaceCriteria(type: CriterionType, newCriteria: Criterion[]) {
    const ofType = this.criteriaFor(type);
    if (ofType.length <= 1) {
      return this.setCriteria([
        ...this.criteria.filter((c) => !ofType.includes(c)),
        ...newCriteria,
      ]);
    }

    const index = this.criteria.indexOf(ofType[0]);
    return this.setCriteria([
      ...this.criteria.slice(0, index),
      ...newCriteria,
      ...this.criteria.slice(index + 1),
    ]);
  }

  public clearCriteria(clearSearchTerm = false) {
    const ret = this.clone();
    if (clearSearchTerm) {
      ret.searchTerm = "";
    }
    ret.criteria = [];
    ret.currentPage = 1;
    return ret;
  }

  public clearSearchTerm() {
    const ret = this.clone();
    ret.searchTerm = "";
    ret.currentPage = 1; // reset to first page
    return ret;
  }

  public setCriteria(criteria: Criterion[]) {
    const ret = this.clone();
    ret.criteria = criteria;
    ret.currentPage = 1; // reset to first page
    return ret;
  }

  public removeCriterion(type: CriterionType) {
    const ret = this.clone();
    const index = ret.criteria.findIndex(
      (cc) => cc.criterionOption.type === type && !inGroup(cc)
    );

    if (index === -1) return ret;

    // only the first of the kind: there can be several
    ret.criteria = ret.criteria.filter((_, i) => i !== index);
    ret.currentPage = 1;
    return ret;
  }

  // Removes the given criterion, which is one of this filter's criteria, and
  // leaves any others of the same kind. A criterion that isn't one of them
  // removes the first of its kind, as removeCriterion does.
  public removeCriterionInstance(c: Criterion) {
    const index = this.criteria.indexOf(c);
    if (index === -1) {
      return this.removeCriterion(c.criterionOption.type);
    }

    const ret = this.clone();
    ret.criteria = ret.criteria.filter((_, i) => i !== index);
    ret.currentPage = 1;
    return ret;
  }

  public removeCustomFieldCriterion(type: CriterionType, index: number) {
    const ret = this.clone();
    const c = ret.criteria.find(
      (cc) => cc.criterionOption.type === type && !inGroup(cc)
    );

    if (!c) return ret;

    if (c instanceof CustomFieldsCriterion) {
      const newCriteria = c.value.filter((_, i) => i !== index);
      c.value = newCriteria;
      // the results change, as when a condition is removed
      ret.currentPage = 1;
    }

    return ret;
  }

  // Removes one value of the given custom fields criterion, which is one of
  // this filter's criteria; there can be several of the kind, and it can be
  // in a group. A criterion that isn't one of them uses the first ungrouped
  // one, as removeCustomFieldCriterion does.
  public removeCustomFieldValue(c: Criterion, index: number) {
    const at = this.criteria.indexOf(c);
    if (at === -1) {
      return this.removeCustomFieldCriterion(c.criterionOption.type, index);
    }

    const ret = this.clone();
    const target = ret.criteria[at];
    if (target instanceof CustomFieldsCriterion) {
      target.value = target.value.filter((_, i) => i !== index);
      ret.currentPage = 1;
    }
    return ret;
  }

  public setPageSize(pageSize: number) {
    const ret = this.clone();
    ret.itemsPerPage = pageSize;
    ret.currentPage = 1; // reset to first page
    return ret;
  }

  public setSortBy(sortBy: string | undefined) {
    const ret = this.clone();
    ret.sortBy = sortBy;
    ret.currentPage = 1; // reset to first page
    return ret;
  }

  public toggleSortDirection() {
    const ret = this.clone();

    if (ret.sortDirection === SortDirectionEnum.Asc) {
      ret.sortDirection = SortDirectionEnum.Desc;
    } else {
      ret.sortDirection = SortDirectionEnum.Asc;
    }

    ret.currentPage = 1; // reset to first page
    return ret;
  }

  public reshuffleRandomSort() {
    const ret = this.clone();
    ret.currentPage = 1;
    ret.randomSeed = -1;
    return ret;
  }

  public changePage(page: number) {
    const ret = this.clone();
    ret.currentPage = page;
    return ret;
  }

  public setZoom(zoomIndex: number) {
    const ret = this.clone();
    ret.zoomIndex = zoomIndex;
    return ret;
  }

  public setDisplayMode(displayMode: DisplayMode) {
    const ret = this.clone();
    ret.displayMode = displayMode;
    return ret;
  }
}
