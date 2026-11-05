import cloneDeep from "lodash-es/cloneDeep";
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Accordion, Button, Card, Form, Modal } from "react-bootstrap";
import cx from "classnames";
import {
  Criterion,
  CriterionOption,
} from "src/models/list-filter/criteria/criterion";
import { FormattedMessage, useIntl } from "react-intl";
import { useConfigurationContext } from "src/hooks/Config";
import {
  ListFilterModel,
  supportsSubFilters,
} from "src/models/list-filter/filter";
import { getFilterOptions } from "src/models/list-filter/factory";
import { FilterTags } from "./FilterTags";
import { CriterionEditor } from "./CriterionEditor";
import { Icon } from "../Shared/Icon";
import {
  faChevronDown,
  faChevronRight,
  faTimes,
  faThumbtack,
} from "@fortawesome/free-solid-svg-icons";
import { useCompare, usePrevious } from "src/hooks/state";
import { CriterionType } from "src/models/list-filter/types";
import { useToast } from "src/hooks/Toast";
import { useConfigureUI, useSaveFilter } from "src/core/StashService";
import {
  FilterMode,
  SavedFilterDataFragment,
} from "src/core/generated-graphql";
import { useFocusOnce } from "src/utils/focus";
import Mousetrap from "mousetrap";
import ScreenUtils from "src/utils/screen";
import {
  LoadFilterDialog,
  notifySavedFilterLoaded,
  SaveFilterDialog,
} from "./SavedFilterList";
import { SearchTermInput } from "./ListFilter";
import { CustomFieldsCriterion } from "src/models/list-filter/criteria/custom-fields";

interface ICriterionList {
  criteria: string[];
  currentCriterion?: Criterion;
  setCriterion: (c: Criterion) => void;
  criterionOptions: CriterionOption[];
  pinnedCriterionOptions: CriterionOption[];
  selected?: CriterionOption;
  optionSelected: (o?: CriterionOption) => void;
  onRemoveCriterion: (c: string) => void;
  onTogglePin: (c: CriterionOption) => void;
  // set when the current criterion is in the filter: starts another
  // condition of the same kind
  onAddAnother?: () => void;
  externallySelected?: boolean;
}

const CriterionOptionList: React.FC<ICriterionList> = ({
  criteria,
  currentCriterion,
  setCriterion,
  criterionOptions,
  pinnedCriterionOptions,
  selected,
  optionSelected,
  onRemoveCriterion,
  onTogglePin,
  onAddAnother,
  externallySelected = false,
}) => {
  const { configuration } = useConfigurationContext();
  const { sfwContentMode } = configuration.interface;

  const prevCriterion = usePrevious(currentCriterion);

  const scrolled = useRef(false);

  const type = currentCriterion?.criterionOption.type;
  const prevType = prevCriterion?.criterionOption.type;

  const criteriaRefs = useMemo(() => {
    const refs: Record<string, React.RefObject<HTMLDivElement>> = {};
    criterionOptions.forEach((c) => {
      refs[c.type] = React.createRef();
    });
    pinnedCriterionOptions.forEach((c) => {
      refs[c.type] = React.createRef();
    });
    return refs;
  }, [criterionOptions, pinnedCriterionOptions]);

  function onSelect(k: string | null) {
    if (!k) {
      optionSelected(undefined);
      return;
    }

    let option = criterionOptions.find((c) => c.type === k);
    if (!option) {
      option = pinnedCriterionOptions.find((c) => c.type === k);
    }

    if (option) {
      optionSelected(option);
    }
  }

  // biome-ignore lint/correctness/useExhaustiveDependencies: intentionally triggering on criterion change
  useEffect(() => {
    // scrolling to the current criterion doesn't work well when the
    // dialog is already open, so limit to when we click on the
    // criterion from the external tags
    if (
      externallySelected &&
      !scrolled.current &&
      type &&
      criteriaRefs[type]?.current
    ) {
      criteriaRefs[type].current!.scrollIntoView({
        behavior: "smooth",
        block: "start",
      });
      scrolled.current = true;
    }
  }, [externallySelected, currentCriterion, criteriaRefs, type]);

  function getReleventCriterion(t: CriterionType) {
    if (currentCriterion?.criterionOption.type === t) {
      return currentCriterion;
    }

    return prevCriterion;
  }

  function removeClicked(ev: React.MouseEvent, t: string) {
    // needed to prevent the nav item from being selected
    ev.stopPropagation();
    ev.preventDefault();
    onRemoveCriterion(t);
  }

  function togglePin(ev: React.MouseEvent, c: CriterionOption) {
    // needed to prevent the nav item from being selected
    ev.stopPropagation();
    ev.preventDefault();
    onTogglePin(c);
  }

  function renderCard(c: CriterionOption, isPin: boolean) {
    return (
      <Card key={c.type} data-type={c.type} ref={criteriaRefs[c.type]!}>
        <Accordion.Toggle className="filter-item-header" eventKey={c.type}>
          <span className="mr-auto">
            <Icon
              className="collapse-icon fa-fw"
              icon={type === c.type ? faChevronDown : faChevronRight}
            />
            <FormattedMessage
              id={
                !sfwContentMode ? c.messageID : (c.sfwMessageID ?? c.messageID)
              }
            />
          </span>
          {criteria.some((cc) => c.type === cc) && (
            <Button
              className="remove-criterion-button"
              variant="minimal"
              onClick={(e) => removeClicked(e, c.type)}
            >
              <Icon icon={faTimes} />
            </Button>
          )}
          <Button
            className="pin-criterion-button"
            variant="minimal"
            onClick={(e) => togglePin(e, c)}
          >
            <Icon icon={faThumbtack} className={isPin ? "" : "tilted"} />
          </Button>
        </Accordion.Toggle>
        <Accordion.Collapse eventKey={c.type}>
          {(type === c.type && currentCriterion) ||
          (prevType === c.type && prevCriterion) ? (
            <Card.Body>
              <CriterionEditor
                criterion={getReleventCriterion(c.type)!}
                setCriterion={setCriterion}
              />
              {type === c.type && onAddAnother && (
                <Button
                  className="add-another-criterion-button"
                  variant="link"
                  size="sm"
                  onClick={() => onAddAnother()}
                >
                  <FormattedMessage id="search_filter.add_another_condition" />
                </Button>
              )}
            </Card.Body>
          ) : (
            <Card.Body></Card.Body>
          )}
        </Accordion.Collapse>
      </Card>
    );
  }

  return (
    <Accordion
      className="criterion-list"
      activeKey={selected?.type}
      onSelect={onSelect}
    >
      {pinnedCriterionOptions.length !== 0 && (
        <>
          {pinnedCriterionOptions.map((c) => renderCard(c, true))}
          <div className="pinned-criterion-divider" />
        </>
      )}
      {criterionOptions.map((c) => renderCard(c, false))}
    </Accordion>
  );
};

const FilterModeToConfigKey = {
  [FilterMode.Audios]: "audios",
  [FilterMode.Galleries]: "galleries",
  [FilterMode.Images]: "images",
  [FilterMode.Movies]: "groups",
  [FilterMode.Groups]: "groups",
  [FilterMode.Performers]: "performers",
  [FilterMode.SceneMarkers]: "sceneMarkers",
  [FilterMode.Scenes]: "scenes",
  [FilterMode.Stories]: "stories",
  [FilterMode.Studios]: "studios",
  [FilterMode.Tags]: "tags",
};

function filterModeToConfigKey(filterMode: FilterMode) {
  return FilterModeToConfigKey[filterMode];
}

interface IEditFilterProps {
  filter: ListFilterModel;
  // the kind of criterion to open, or one of filter's criteria
  editingCriterion?: string | Criterion;
  onApply: (filter: ListFilterModel) => void;
  onCancel: () => void;
}

export const EditFilterDialog: React.FC<IEditFilterProps> = ({
  filter,
  editingCriterion,
  onApply,
  onCancel,
}) => {
  const Toast = useToast();
  const intl = useIntl();

  const { configuration } = useConfigurationContext();
  const { sfwContentMode } = configuration.interface;

  const [searchValue, setSearchValue] = useState("");
  const [currentFilter, setCurrentFilter] = useState<ListFilterModel>(
    cloneDeep(filter)
  );
  const [criterion, setCriterion] = useState<Criterion>();
  // where criterion is in currentFilter.criteria; undefined while it is a new
  // condition not added yet. There can be several conditions of one kind.
  const [editingIndex, setEditingIndex] = useState<number>();

  const [searchRef, setSearchFocus] = useFocusOnce(!ScreenUtils.isTouch());

  const [showSaveDialog, setShowSaveDialog] = useState(false);
  const [savingFilter, setSavingFilter] = useState(false);

  const [showLoadDialog, setShowLoadDialog] = useState(false);

  const saveFilter = useSaveFilter();

  const { criteria } = currentFilter;

  const criteriaList = useMemo(() => {
    return criteria.map((c) => c.criterionOption.type);
  }, [criteria]);

  const filterOptions = useMemo(() => {
    return getFilterOptions(currentFilter.mode);
  }, [currentFilter.mode]);

  const criterionOptions = useMemo(() => {
    return [...filterOptions.criterionOptions]
      .filter((c) => !c.hidden)
      .sort((a, b) => {
        return intl
          .formatMessage({
            id: !sfwContentMode ? a.messageID : (a.sfwMessageID ?? a.messageID),
          })
          .localeCompare(
            intl.formatMessage({
              id: !sfwContentMode
                ? b.messageID
                : (b.sfwMessageID ?? b.messageID),
            })
          );
      });
  }, [intl, sfwContentMode, filterOptions.criterionOptions]);

  const optionSelected = useCallback(
    (option?: CriterionOption) => {
      if (!option) {
        setCriterion(undefined);
        setEditingIndex(undefined);
        return;
      }

      // find the existing criterion if present
      const existing = criteria.findIndex(
        (c) => c.criterionOption.type === option.type
      );
      if (existing !== -1) {
        setCriterion(criteria[existing]);
        setEditingIndex(existing);
      } else {
        const newCriterion = filter.makeCriterion(option.type);
        setCriterion(newCriterion);
        setEditingIndex(undefined);
      }
    },
    [filter, criteria]
  );

  // edits the criterion at index in currentFilter.criteria
  const editCriterionAt = useCallback(
    (index: number) => {
      setCriterion(criteria[index]);
      setEditingIndex(index);
    },
    [criteria]
  );

  function addAnother() {
    if (!criterion) return;
    setCriterion(filter.makeCriterion(criterion.criterionOption.type));
    setEditingIndex(undefined);
  }

  const ui = configuration?.ui ?? {};
  const [saveUI] = useConfigureUI();

  const filteredOptions = useMemo(() => {
    const trimmedSearch = searchValue.trim().toLowerCase();
    if (!trimmedSearch) {
      return criterionOptions;
    }

    return criterionOptions.filter((c) => {
      return intl
        .formatMessage({
          id: !sfwContentMode ? c.messageID : (c.sfwMessageID ?? c.messageID),
        })
        .toLowerCase()
        .includes(trimmedSearch);
    });
  }, [intl, sfwContentMode, searchValue, criterionOptions]);

  const pinnedFilters = useMemo(
    () => ui.pinnedFilters?.[filterModeToConfigKey(currentFilter.mode)] ?? [],
    [currentFilter.mode, ui.pinnedFilters]
  );
  const pinnedElements = useMemo(
    () => filteredOptions.filter((c) => pinnedFilters.includes(c.messageID)),
    [pinnedFilters, filteredOptions]
  );
  const unpinnedElements = useMemo(
    () => filteredOptions.filter((c) => !pinnedFilters.includes(c.messageID)),
    [pinnedFilters, filteredOptions]
  );

  const editingCriterionChanged = useCompare(editingCriterion);

  useEffect(() => {
    if (editingCriterionChanged && editingCriterion) {
      if (typeof editingCriterion !== "string") {
        // currentFilter's criteria are copies of filter's, in the same order
        const index = filter.criteria.indexOf(editingCriterion);
        if (index !== -1) {
          editCriterionAt(index);
          return;
        }
      }

      const type =
        typeof editingCriterion === "string"
          ? editingCriterion
          : editingCriterion.criterionOption.type;
      const option = criterionOptions.find((c) => c.type === type);
      if (option) {
        optionSelected(option);
      }
    }
  }, [
    filter,
    editingCriterion,
    criterionOptions,
    optionSelected,
    editCriterionAt,
    editingCriterionChanged,
  ]);

  useEffect(() => {
    Mousetrap.bind("/", (e) => {
      setSearchFocus();
      e.preventDefault();
    });

    return () => {
      Mousetrap.unbind("/");
    };
  });

  async function updatePinnedFilters(filters: string[]) {
    const configKey = filterModeToConfigKey(currentFilter.mode);
    try {
      await saveUI({
        variables: {
          input: {
            ...configuration?.ui,
            pinnedFilters: {
              ...ui.pinnedFilters,
              [configKey]: filters,
            },
          },
        },
      });
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onTogglePinFilter(f: CriterionOption) {
    try {
      const existing = pinnedFilters.find((name) => name === f.messageID);
      if (existing) {
        await updatePinnedFilters(
          pinnedFilters.filter((name) => name !== f.messageID)
        );
      } else {
        await updatePinnedFilters([...pinnedFilters, f.messageID]);
      }
    } catch (err) {
      Toast.error(err);
    }
  }

  function replaceCriterion(c: Criterion) {
    const newFilter = cloneDeep(currentFilter);
    let newIndex = editingIndex;

    if (!c.isValid()) {
      // remove from the filter if present
      if (editingIndex !== undefined) {
        newFilter.criteria = criteria.filter((_, i) => i !== editingIndex);
      }
      newIndex = undefined;
    } else if (editingIndex !== undefined) {
      newFilter.criteria = criteria.map((cc, i) =>
        i === editingIndex ? c : cc
      );
    } else {
      newFilter.criteria = [...criteria, c];
      newIndex = newFilter.criteria.length - 1;
    }

    setCurrentFilter(newFilter);
    setCriterion(c);
    setEditingIndex(newIndex);
  }

  // removes the given criteria, which are in currentFilter.criteria
  function removeCriteria(removed: Criterion[]) {
    const newFilter = cloneDeep(currentFilter);
    newFilter.criteria = criteria.filter((cc) => !removed.includes(cc));
    setCurrentFilter(newFilter);

    if (editingIndex === undefined) return;
    if (removed.includes(criteria[editingIndex])) {
      optionSelected(undefined);
    } else {
      // the edited criterion moves up past those removed before it
      const before = criteria
        .slice(0, editingIndex)
        .filter((cc) => removed.includes(cc)).length;
      setEditingIndex(editingIndex - before);
    }
  }

  function removeCriterion(c: Criterion, valueIndex?: number) {
    if (valueIndex !== undefined) {
      setCurrentFilter(
        currentFilter.removeCustomFieldCriterion(
          c.criterionOption.type,
          valueIndex
        )
      );
    } else {
      removeCriteria([c]);
    }
  }

  // the remove button of a kind of criterion removes all of that kind
  function removeCriterionString(c: string) {
    removeCriteria(criteria.filter((cc) => cc.criterionOption.type === c));
  }

  function onClearAll() {
    const newFilter = cloneDeep(currentFilter);
    newFilter.criteria = [];
    setCurrentFilter(newFilter);
    // an open condition is now a new one
    setEditingIndex(undefined);
  }

  function onLoadFilter(f: SavedFilterDataFragment) {
    const newFilter = filter.clone();

    newFilter.currentPage = 1;
    // #1795 - reset search term if not present in saved filter
    newFilter.searchTerm = "";
    newFilter.configureFromSavedFilter(f);
    // #1507 - reset random seed when loaded
    newFilter.randomSeed = -1;

    onApply(newFilter);
    notifySavedFilterLoaded({
      filter: newFilter,
      savedFilter: f,
      source: "dialog",
    });
  }

  async function onSaveFilter(name: string, id?: string) {
    try {
      setSavingFilter(true);
      await saveFilter(filter, name, id);

      Toast.success(
        intl.formatMessage(
          {
            id: "toast.saved_entity",
          },
          {
            entity: intl.formatMessage({ id: "filter" }).toLocaleLowerCase(),
          }
        )
      );
      setShowSaveDialog(false);
      onApply(currentFilter);
    } catch (err) {
      Toast.error(err);
    } finally {
      setSavingFilter(false);
    }
  }

  return (
    <>
      {showSaveDialog && (
        <SaveFilterDialog
          mode={filter.mode}
          onClose={(name, id) => {
            if (name) {
              onSaveFilter(name, id);
            } else {
              setShowSaveDialog(false);
            }
          }}
          isSaving={savingFilter}
        />
      )}
      {showLoadDialog && (
        <LoadFilterDialog
          mode={filter.mode}
          onClose={(f) => {
            if (f) {
              onLoadFilter(f);
            }
            setShowLoadDialog(false);
          }}
        />
      )}
      <Modal
        show={!showSaveDialog && !showLoadDialog}
        onHide={() => onCancel()}
        // need sfw mode class because dialog is outside body
        className={cx("edit-filter-dialog", {
          "sfw-content-mode": sfwContentMode,
        })}
      >
        <Modal.Header>
          <div>
            <FormattedMessage id="search_filter.edit_filter" />
          </div>
          <Form.Control
            className="btn-secondary search-input"
            onChange={(e) => setSearchValue(e.target.value)}
            value={searchValue}
            placeholder={`${intl.formatMessage({ id: "actions.search" })}…`}
            ref={searchRef}
          />
        </Modal.Header>
        <Modal.Body>
          <div
            className={cx("dialog-content", {
              "criterion-selected": !!criterion,
            })}
          >
            <div className="search-term-row">
              <span>
                <FormattedMessage id="search_filter.search_term" />
              </span>
              <SearchTermInput
                filter={currentFilter}
                onFilterUpdate={setCurrentFilter}
              />
            </div>
            <CriterionOptionList
              criteria={criteriaList}
              currentCriterion={criterion}
              setCriterion={replaceCriterion}
              criterionOptions={unpinnedElements}
              pinnedCriterionOptions={pinnedElements}
              optionSelected={optionSelected}
              selected={criterion?.criterionOption}
              onRemoveCriterion={(c) => removeCriterionString(c)}
              onTogglePin={(c) => onTogglePinFilter(c)}
              onAddAnother={
                // custom fields already hold several conditions, and some
                // lists can't send repeats
                editingIndex !== undefined &&
                !(criterion instanceof CustomFieldsCriterion) &&
                supportsSubFilters(currentFilter.mode)
                  ? addAnother
                  : undefined
              }
              externallySelected={!!editingCriterion}
            />
            {criteria.length > 0 && (
              <div>
                <FilterTags
                  criteria={criteria}
                  onEditCriterion={(c) => {
                    const index = criteria.indexOf(c);
                    if (index !== -1) {
                      editCriterionAt(index);
                    } else {
                      optionSelected(c.criterionOption);
                    }
                  }}
                  onRemoveCriterion={removeCriterion}
                  onRemoveAll={() => onClearAll()}
                />
              </div>
            )}
          </div>
        </Modal.Body>
        <Modal.Footer>
          <div>
            <Button
              variant="secondary"
              onClick={() => setShowLoadDialog(true)}
              title={intl.formatMessage({ id: "actions.load_filter" })}
            >
              <FormattedMessage id="actions.load" />…
            </Button>
            <Button
              variant="secondary"
              onClick={() => setShowSaveDialog(true)}
              title={intl.formatMessage({ id: "actions.save_filter" })}
            >
              <FormattedMessage id="actions.save" />…
            </Button>
          </div>
          <div>
            <Button variant="secondary" onClick={() => onCancel()}>
              <FormattedMessage id="actions.cancel" />
            </Button>
            <Button onClick={() => onApply(currentFilter)}>
              <FormattedMessage id="actions.apply" />
            </Button>
          </div>
        </Modal.Footer>
      </Modal>
    </>
  );
};

export function useShowEditFilter(props: {
  filter: ListFilterModel;
  setFilter: (f: ListFilterModel) => void;
  showModal: (content: React.ReactNode) => void;
  closeModal: () => void;
}) {
  const { filter, setFilter, showModal, closeModal } = props;

  const showEditFilter = useCallback(
    (editingCriterion?: string | Criterion) => {
      function onApplyEditFilter(f: ListFilterModel) {
        closeModal();
        setFilter(f);
      }

      showModal(
        <EditFilterDialog
          filter={filter}
          onApply={onApplyEditFilter}
          onCancel={() => closeModal()}
          editingCriterion={editingCriterion}
        />
      );
    },
    [filter, setFilter, showModal, closeModal]
  );

  return showEditFilter;
}
