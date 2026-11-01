import { createContext, useContext, useEffect, useMemo, useRef } from "react";
import Mousetrap from "mousetrap";

interface ListActions {
  hasSelection: boolean;
  onEdit?: () => void;
  onDelete?: () => void;
}

export const PageListShortcutsContext = createContext<
  | {
      selection: React.MutableRefObject<ListActions | undefined>;
      favorite: boolean;
    }
  | undefined
>(undefined);

// The detail page owns the bindings. Lists supply actions without replacing them.
export function usePageListShortcuts(actions: {
  onEdit?: () => void;
  onDelete?: () => void;
  onFavorite?: () => void;
}) {
  const selection = useRef<ListActions>();
  const { onEdit, onDelete, onFavorite } = actions;

  useEffect(() => {
    Mousetrap.bind("e", () => {
      if (selection.current?.hasSelection) selection.current.onEdit?.();
      else onEdit?.();
    });
    Mousetrap.bind("d d", () => {
      if (selection.current?.hasSelection) selection.current.onDelete?.();
      else onDelete?.();
    });
    if (onFavorite) Mousetrap.bind("f", onFavorite);

    return () => {
      Mousetrap.unbind("e");
      Mousetrap.unbind("d d");
      if (onFavorite) Mousetrap.unbind("f");
    };
  }, [onEdit, onDelete, onFavorite]);

  const favorite = !!onFavorite;
  return useMemo(() => ({ selection, favorite }), [favorite]);
}

export function useListOperationShortcuts(actions: ListActions) {
  const page = useContext(PageListShortcutsContext);
  const selection = page?.selection;
  const { hasSelection, onEdit, onDelete } = actions;

  useEffect(() => {
    if (selection) {
      selection.current = { hasSelection, onEdit, onDelete };
      return () => {
        selection.current = undefined;
      };
    }

    Mousetrap.bind("e", () => {
      if (hasSelection) onEdit?.();
    });
    Mousetrap.bind("d d", () => {
      if (hasSelection) onDelete?.();
    });
    return () => {
      Mousetrap.unbind("e");
      Mousetrap.unbind("d d");
    };
  }, [selection, hasSelection, onEdit, onDelete]);
}

export function useListFilterShortcut(showEditFilter?: () => void) {
  const page = useContext(PageListShortcutsContext);
  const pageHasFavorite = page?.favorite;

  useEffect(() => {
    if (!showEditFilter || pageHasFavorite) return;

    Mousetrap.bind("f", (event) => {
      showEditFilter();
      // The dialog focuses its query field; don't type f into it.
      event.preventDefault();
    });
    return () => {
      Mousetrap.unbind("f");
    };
  }, [showEditFilter, pageHasFavorite]);
}
