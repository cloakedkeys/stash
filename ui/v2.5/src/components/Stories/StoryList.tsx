import React from "react";
import { FormattedMessage, useIntl } from "react-intl";
import cx from "classnames";
import { Button } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import { objectTitle } from "src/core/files";
import { useFindStories, useStoriesDestroy } from "src/core/StashService";
import { ListFilterModel } from "src/models/list-filter/filter";
import { useFilteredItemList } from "../List/ItemList";
import { DeleteEntityDialog } from "../Shared/DeleteEntityDialog";
import { View } from "../List/views";
import { FilteredListToolbar } from "../List/FilteredListToolbar";
import {
  Sidebar,
  SidebarPane,
  SidebarPaneContent,
  SidebarStateContext,
  useSidebarState,
} from "../Shared/Sidebar";
import { useCloseEditDelete, useFilterOperations } from "../List/util";
import {
  FilteredSidebarHeader,
  useFilteredSidebarKeybinds,
} from "../List/Filters/FilterSidebar";
import { ListOperations } from "../List/ListOperationButtons";
import { FilterTags } from "../List/FilterTags";
import { Pagination, PaginationIndex } from "../List/Pagination";
import { LoadedContent } from "../List/PagedList";
import { SidebarTagsFilter } from "../List/Filters/TagsFilter";
import { SidebarRatingFilter } from "../List/Filters/RatingFilter";
import useFocus from "src/utils/focus";
import { StoryListTable } from "./StoryListTable";

const SidebarContent: React.FC<{
  filter: ListFilterModel;
  setFilter: (filter: ListFilterModel) => void;
  view?: View;
  sidebarOpen: boolean;
  onClose?: () => void;
  showEditFilter: (editingCriterion?: string) => void;
  count?: number;
  focus?: ReturnType<typeof useFocus>;
}> = ({
  filter,
  setFilter,
  view,
  showEditFilter,
  sidebarOpen,
  onClose,
  count,
  focus,
}) => {
  const showResultsId =
    count !== undefined ? "actions.show_count_results" : "actions.show_results";

  return (
    <>
      <FilteredSidebarHeader
        sidebarOpen={sidebarOpen}
        showEditFilter={showEditFilter}
        filter={filter}
        setFilter={setFilter}
        view={view}
        focus={focus}
      />

      <SidebarTagsFilter filter={filter} setFilter={setFilter} />
      <SidebarRatingFilter filter={filter} setFilter={setFilter} />

      <div className="sidebar-footer">
        <Button className="sidebar-close-button" onClick={onClose}>
          <FormattedMessage id={showResultsId} values={{ count }} />
        </Button>
      </div>
    </>
  );
};

interface IStoryList {
  view?: View;
  alterQuery?: boolean;
}

export const StoryList: React.FC<IStoryList> = ({ view, alterQuery }) => {
  const intl = useIntl();
  const searchFocus = useFocus();

  const {
    showSidebar,
    setShowSidebar,
    sectionOpen,
    setSectionOpen,
    loading: sidebarStateLoading,
  } = useSidebarState(view);

  const { filterState, queryResult, modalState, listSelect, showEditFilter } =
    useFilteredItemList({
      filterStateProps: {
        filterMode: GQL.FilterMode.Stories,
        view,
        useURL: alterQuery,
      },
      queryResultProps: {
        useResult: useFindStories,
        getCount: (r) => r.data?.findStories.count ?? 0,
        getItems: (r) => r.data?.findStories.stories ?? [],
      },
    });

  const { filter, setFilter } = filterState;
  const { result, cachedResult, items, totalCount } = queryResult;
  const { selectedIds, selectedItems, onSelectChange, onSelectNone } =
    listSelect;
  const { modal, showModal, closeModal } = modalState;

  const { setPage, removeCriterion, clearAllCriteria } = useFilterOperations({
    filter,
    setFilter,
  });

  useFilteredSidebarKeybinds({ showSidebar, setShowSidebar });

  const onCloseEditDelete = useCloseEditDelete({
    closeModal,
    onSelectNone,
    result,
  });

  function onDelete() {
    showModal(
      <DeleteEntityDialog
        selected={selectedItems.map((s) => ({
          id: s.id,
          name: objectTitle(s),
        }))}
        onClose={onCloseEditDelete}
        singularEntity={intl.formatMessage({ id: "story" })}
        pluralEntity={intl.formatMessage({ id: "stories" })}
        destroyMutation={useStoriesDestroy}
      />
    );
  }

  if (sidebarStateLoading) return null;

  const operations = (
    <ListOperations
      items={items.length}
      hasSelection={listSelect.hasSelection}
      operations={[
        {
          text: intl.formatMessage({ id: "actions.select_all" }),
          onClick: () => listSelect.onSelectAll(),
          isDisplayed: () => totalCount > 0,
        },
        {
          text: intl.formatMessage({ id: "actions.select_none" }),
          onClick: () => onSelectNone(),
          isDisplayed: () => listSelect.hasSelection,
        },
      ]}
      onDelete={onDelete}
      operationsMenuClassName="story-list-operations-dropdown"
    />
  );

  return (
    <div
      className={cx("item-list-container story-list", {
        "hide-sidebar": !showSidebar,
      })}
    >
      {modal}

      <SidebarStateContext.Provider value={{ sectionOpen, setSectionOpen }}>
        <SidebarPane hideSidebar={!showSidebar}>
          <Sidebar hide={!showSidebar} onHide={() => setShowSidebar(false)}>
            <SidebarContent
              filter={filter}
              setFilter={setFilter}
              showEditFilter={showEditFilter}
              view={view}
              sidebarOpen={showSidebar}
              onClose={() => setShowSidebar(false)}
              count={cachedResult.loading ? undefined : totalCount}
              focus={searchFocus}
            />
          </Sidebar>
          <SidebarPaneContent
            onSidebarToggle={() => setShowSidebar(!showSidebar)}
          >
            <FilteredListToolbar
              filter={filter}
              listSelect={listSelect}
              setFilter={setFilter}
              showEditFilter={showEditFilter}
              onDelete={onDelete}
              operationComponent={operations}
              view={view}
            />

            <FilterTags
              view={view}
              criteria={filter.criteria}
              onEditCriterion={(c) => showEditFilter(c.criterionOption.type)}
              onRemoveCriterion={removeCriterion}
              onRemoveAll={clearAllCriteria}
            />

            <div className="pagination-index-container">
              <Pagination
                currentPage={filter.currentPage}
                itemsPerPage={filter.itemsPerPage}
                totalItems={totalCount}
                onChangePage={(page) => setFilter(filter.changePage(page))}
              />
              <PaginationIndex
                loading={cachedResult.loading}
                itemsPerPage={filter.itemsPerPage}
                currentPage={filter.currentPage}
                totalItems={totalCount}
              />
            </div>

            <LoadedContent loading={result.loading} error={result.error}>
              {items.length > 0 && (
                <StoryListTable
                  stories={items}
                  selectedIds={selectedIds}
                  onSelectChange={onSelectChange}
                />
              )}
            </LoadedContent>

            {totalCount > filter.itemsPerPage && (
              <div className="pagination-footer-container">
                <div className="pagination-footer">
                  <Pagination
                    itemsPerPage={filter.itemsPerPage}
                    currentPage={filter.currentPage}
                    totalItems={totalCount}
                    onChangePage={setPage}
                    pagePopupPlacement="top"
                  />
                </div>
              </div>
            )}
          </SidebarPaneContent>
        </SidebarPane>
      </SidebarStateContext.Provider>
    </div>
  );
};

export default StoryList;
