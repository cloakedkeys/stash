import { useConfigurationContext } from "src/hooks/Config";
import { useConfigureUISetting } from "src/core/StashService";
import { useToast } from "src/hooks/Toast";

export function sortPinnedScrapers<T extends { id: string }>(
  scrapers: T[],
  pinned: string[] = []
): T[] {
  const rank = (id: string) => {
    const index = pinned.indexOf(id);
    return index < 0 ? pinned.length : index;
  };
  return [...scrapers].sort((a, b) => rank(a.id) - rank(b.id));
}

export function usePinnedScrapers() {
  const { configuration } = useConfigurationContext();
  const pinnedScrapers = configuration.ui.pinnedScrapers ?? [];
  const [saveUISetting, { loading }] = useConfigureUISetting();
  const Toast = useToast();

  async function togglePin(id: string) {
    const value = pinnedScrapers.includes(id)
      ? pinnedScrapers.filter((s) => s !== id)
      : [...pinnedScrapers, id];
    try {
      await saveUISetting({ variables: { key: "pinnedScrapers", value } });
    } catch (e) {
      Toast.error(e);
    }
  }

  return { pinnedScrapers, togglePin, saving: loading };
}
