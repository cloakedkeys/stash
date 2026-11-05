import { faCheck, faList, faTimes } from "@fortawesome/free-solid-svg-icons";
import React, { useState } from "react";
import { Button, Row, Col } from "react-bootstrap";
import { useIntl } from "react-intl";

import { ModalComponent } from "../Shared/Modal";
import { Icon } from "../Shared/Icon";
import { listToMap } from "src/utils/data";

interface IProps {
  show: boolean;
  fields: string[];
  excludedFields: string[];
  onSelect: (fields: string[]) => void;
  onCancel?: () => void;
}

const FieldSelector: React.FC<IProps> = ({
  show,
  fields,
  excludedFields,
  onSelect,
  onCancel,
}) => {
  const intl = useIntl();
  const savedExcluded = () =>
    listToMap(excludedFields.filter((field) => fields.includes(field)));
  const [excluded, setExcluded] =
    useState<Record<string, boolean>>(savedExcluded);

  // drop the unsaved toggles, so the dialog shows the saved fields next time
  const cancel = onCancel
    ? () => {
        setExcluded(savedExcluded());
        onCancel();
      }
    : undefined;

  const toggleField = (field: string) =>
    setExcluded({
      ...excluded,
      [field]: !excluded[field],
    });

  const renderField = (field: string) => (
    <Col xs={6} className="mb-1" key={field}>
      <Button
        onClick={() => toggleField(field)}
        variant="secondary"
        className={excluded[field] ? "text-muted" : "text-success"}
      >
        <Icon icon={excluded[field] ? faTimes : faCheck} />
      </Button>
      <span className="ml-3">{intl.formatMessage({ id: field })}</span>
    </Col>
  );

  return (
    <ModalComponent
      show={show}
      icon={faList}
      dialogClassName="FieldSelect"
      cancel={cancel && { onClick: cancel, variant: "secondary" }}
      accept={{
        text: intl.formatMessage({ id: "actions.save" }),
        onClick: () =>
          onSelect(Object.keys(excluded).filter((f) => excluded[f])),
      }}
    >
      <h4>Select tagged fields</h4>
      <div className="mb-2">
        These fields will be tagged by default. Click the button to toggle.
      </div>
      <Row>{fields.map((f) => renderField(f))}</Row>
    </ModalComponent>
  );
};

export default FieldSelector;
