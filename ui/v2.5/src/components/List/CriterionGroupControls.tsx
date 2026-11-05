import React from "react";
import { Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { Criterion } from "src/models/list-filter/criteria/criterion";
import {
  canMatchEmpty,
  inGroup,
  ListFilterModel,
  supportsSubFilters,
} from "src/models/list-filter/filter";

const NEW_ANY = "new-any";
const NEW_NONE = "new-none";

interface ICriterionGroupControls {
  // the filter the criterion is in
  filter: ListFilterModel;
  criterion: Criterion;
  onChange: (change: (c: Criterion) => void) => void;
}

// Moves a condition into an Any of / None of group or back (#1206), and sets
// whether it also matches empty values (#1170).
export const CriterionGroupControls: React.FC<ICriterionGroupControls> = ({
  filter,
  criterion,
  onChange,
}) => {
  const intl = useIntl();

  // groups and "or empty" are sent with AND/OR/NOT/ALL_OF, which some lists
  // (Markers) don't have
  if (!supportsSubFilters(filter.mode)) return null;

  const groups = filter.groups();
  const current = inGroup(criterion) ? criterion.filterGroup! : "";

  function onGroupChange(value: string) {
    let name: string | undefined = value || undefined;
    if (value === NEW_ANY) name = filter.newGroupName("any");
    if (value === NEW_NONE) name = filter.newGroupName("none");
    onChange((c) => {
      c.filterGroup = name;
    });
  }

  return (
    <div className="criterion-group-controls">
      <Form.Group controlId="criterion-group" className="mb-1">
        <Form.Label className="mr-2">
          <FormattedMessage id="search_filter.condition_match" />
        </Form.Label>
        <Form.Control
          as="select"
          size="sm"
          className="input-control"
          value={current}
          onChange={(e) => onGroupChange(e.currentTarget.value)}
        >
          <option value="">
            {intl.formatMessage({ id: "search_filter.all_of_conditions" })}
          </option>
          {groups.map((g) => (
            <option key={g.name} value={g.name}>
              {intl.formatMessage(
                {
                  id:
                    g.kind === "none"
                      ? "search_filter.none_of_group"
                      : "search_filter.any_of_group",
                },
                { number: g.number }
              )}
            </option>
          ))}
          <option value={NEW_ANY}>
            {intl.formatMessage({ id: "search_filter.new_any_of_group" })}
          </option>
          <option value={NEW_NONE}>
            {intl.formatMessage({ id: "search_filter.new_none_of_group" })}
          </option>
        </Form.Control>
      </Form.Group>
      {canMatchEmpty(criterion) && (
        <Form.Check
          id="criterion-or-empty"
          checked={!!criterion.orEmpty}
          label={intl.formatMessage({ id: "search_filter.or_empty" })}
          onChange={(e) => {
            const { checked } = e.currentTarget;
            onChange((c) => {
              c.orEmpty = checked || undefined;
            });
          }}
        />
      )}
    </div>
  );
};
