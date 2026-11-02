import * as GQL from "src/core/generated-graphql";

// Saved filters the front page has no row for yet, see Control.tsx.
const unsupportedModes: GQL.FilterMode[] = [GQL.FilterMode.Stories];

// The saved filters that can be added to the front page: ones it can show,
// that aren't on it already.
export function frontPageFilterCandidates<
  T extends { id: string; mode: GQL.FilterMode },
>(filters: readonly T[], existingIDs: readonly string[]): T[] {
  return filters.filter(
    (f) => !existingIDs.includes(f.id) && !unsupportedModes.includes(f.mode)
  );
}
