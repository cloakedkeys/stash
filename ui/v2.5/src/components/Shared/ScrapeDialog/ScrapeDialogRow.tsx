import React, { useContext, useEffect, useRef, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import {
  Form,
  Col,
  Row,
  InputGroup,
  Button,
  FormControl,
} from "react-bootstrap";
import { Icon } from "../Icon";
import { faCheck, faTimes } from "@fortawesome/free-solid-svg-icons";
import { getCountryByISO } from "src/utils/country";
import { CountrySelect } from "../CountrySelect";
import { StringListInput } from "../StringListInput";
import { ImageSelector } from "../ImageSelector";
import {
  CustomFieldScrapeResults,
  mergeLists,
  ScrapeResult,
} from "./scrapeResult";
import { ScrapeDialogContext } from "./ScrapeDialog";

function renderButtonIcon(selected: boolean) {
  const className = selected ? "text-success" : "text-muted";

  return (
    <Icon
      className={`fa-fw ${className}`}
      icon={selected ? faCheck : faTimes}
    />
  );
}

interface IScrapedFieldProps<T> {
  result: ScrapeResult<T>;
}

interface IScrapedRowProps<T> extends IScrapedFieldProps<T> {
  className?: string;
  field: string;
  title: string;
  originalField: React.ReactNode;
  newField: React.ReactNode;
  onChange: (value: ScrapeResult<T>) => void;
  newValues?: React.ReactNode;
  alwaysShow?: boolean;
  mergeValues?: (original?: T, scraped?: T) => T;
  mergedField?: (value: T) => React.ReactNode;
}

export const ScrapeDialogRow = <T,>(props: IScrapedRowProps<T>) => {
  const intl = useIntl();
  const intlMergeTitle = intl.formatMessage({
    id: "dialogs.scrape_merge_description",
  });
  const {
    existingLabel,
    scrapedLabel,
    reverseColumns,
    mergeDefaults,
    setMergeDefault,
  } = useContext(ScrapeDialogContext);
  const initialized = useRef(false);
  const canMerge = !!props.mergeValues && !!mergeDefaults;

  useEffect(() => {
    // Missing objects may only become selectable after the user links them.
    if (initialized.current || !canMerge || !props.result.scraped) return;
    initialized.current = true;
    if (mergeDefaults?.[props.field]) {
      props.onChange(props.result.cloneWithMerge(props.mergeValues!));
    }
  }, [
    canMerge,
    mergeDefaults,
    props.field,
    props.result,
    props.mergeValues,
    props.onChange,
  ]);

  function handleSelectClick(isNew: boolean) {
    props.onChange(props.result.cloneWithSelection(isNew));
  }

  if (!props.result.scraped && !props.newValues && !props.alwaysShow) {
    return null;
  }

  const existingField = (
    <React.Fragment key="existing">
      <Form.Label column className="d-lg-none column-label">
        {existingLabel}
      </Form.Label>
      <Col lg="6">
        <InputGroup>
          <InputGroup.Prepend className="bg-secondary text-white border-secondary">
            <Button
              variant="secondary"
              onClick={() => handleSelectClick(false)}
            >
              {renderButtonIcon(
                !props.result.useNewValue && !props.result.useMergedValue
              )}
            </Button>
          </InputGroup.Prepend>
          {props.originalField}
        </InputGroup>
      </Col>
    </React.Fragment>
  );
  const scrapedField = (
    <React.Fragment key="scraped">
      <Form.Label column className="d-lg-none column-label">
        {scrapedLabel}
      </Form.Label>
      <Col lg="6">
        <InputGroup>
          <InputGroup.Prepend>
            <Button variant="secondary" onClick={() => handleSelectClick(true)}>
              {renderButtonIcon(
                props.result.useNewValue && !props.result.useMergedValue
              )}
            </Button>
          </InputGroup.Prepend>
          {props.newField}
        </InputGroup>
        {props.newValues}
      </Col>
    </React.Fragment>
  );

  return (
    <Row
      className={`px-3 pt-3 ${props.className ?? ""}`}
      data-field={props.field}
    >
      <Form.Label column lg="3">
        {props.title}
        {canMerge && (
          <div>
            <Button
              size="sm"
              variant={props.result.useMergedValue ? "primary" : "secondary"}
              aria-pressed={props.result.useMergedValue}
              title={intlMergeTitle}
              onClick={() =>
                props.onChange(props.result.cloneWithMerge(props.mergeValues!))
              }
            >
              <FormattedMessage id="actions.merge" />
            </Button>
            <Form.Check
              id={`scrape-merge-default-${props.field}`}
              className="mt-2"
              checked={mergeDefaults?.[props.field] ?? false}
              onChange={(e) =>
                setMergeDefault?.(props.field, e.currentTarget.checked)
              }
              label={<FormattedMessage id="dialogs.scrape_merge_default" />}
            />
          </div>
        )}
      </Form.Label>

      <Col lg="9">
        <Row>
          {reverseColumns
            ? [scrapedField, existingField]
            : [existingField, scrapedField]}
        </Row>
        {canMerge && props.result.useMergedValue && props.mergedField && (
          <div className="mt-2" aria-live="polite">
            <Form.Label>
              <FormattedMessage id="actions.merge" />
            </Form.Label>
            {props.mergedField(props.result.currentValue()!)}
          </div>
        )}
      </Col>
    </Row>
  );
};

interface IScrapedInputGroupProps {
  isNew?: boolean;
  placeholder?: string;
  locked?: boolean;
  result: ScrapeResult<string>;
  onChange?: (value: string) => void;
}

const ScrapedInputGroup: React.FC<IScrapedInputGroupProps> = (props) => {
  return (
    <FormControl
      placeholder={props.placeholder}
      value={props.isNew ? props.result.newValue : props.result.originalValue}
      readOnly={!props.isNew || props.locked}
      onChange={(e) => {
        if (props.isNew && props.onChange) {
          props.onChange(e.target.value);
        }
      }}
      className="bg-secondary text-white border-secondary"
    />
  );
};

interface IScrapedInputGroupRowProps {
  mergeValues?: (original?: string, scraped?: string) => string;
  title: string;
  field: string;
  className?: string;
  placeholder?: string;
  result: ScrapeResult<string>;
  locked?: boolean;
  onChange: (value: ScrapeResult<string>) => void;
}

export const ScrapedInputGroupRow: React.FC<IScrapedInputGroupRowProps> = (
  props
) => {
  return (
    <ScrapeDialogRow
      title={props.title}
      field={props.field}
      className={props.className}
      result={props.result}
      originalField={
        <ScrapedInputGroup
          placeholder={props.placeholder || props.title}
          result={props.result}
        />
      }
      newField={
        <ScrapedInputGroup
          placeholder={props.placeholder || props.title}
          result={props.result}
          isNew
          locked={props.locked}
          onChange={(value) =>
            props.onChange(props.result.cloneWithValue(value))
          }
        />
      }
      onChange={props.onChange}
    />
  );
};

interface IScrapedNumberInputProps {
  isNew?: boolean;
  placeholder?: string;
  locked?: boolean;
  result: ScrapeResult<number>;
  onChange?: (value: number) => void;
}

const ScrapedNumberInput: React.FC<IScrapedNumberInputProps> = (props) => {
  return (
    <FormControl
      placeholder={props.placeholder}
      value={props.isNew ? props.result.newValue : props.result.originalValue}
      readOnly={!props.isNew || props.locked}
      onChange={(e) => {
        if (props.isNew && props.onChange) {
          props.onChange(Number(e.target.value));
        }
      }}
      className="bg-secondary text-white border-secondary"
      type="number"
    />
  );
};

interface IScrapedNumberRowProps {
  title: string;
  field: string;
  className?: string;
  placeholder?: string;
  result: ScrapeResult<number>;
  locked?: boolean;
  onChange: (value: ScrapeResult<number>) => void;
}

export const ScrapedNumberRow: React.FC<IScrapedNumberRowProps> = (props) => {
  return (
    <ScrapeDialogRow
      title={props.title}
      field={props.field}
      className={props.className}
      result={props.result}
      originalField={
        <ScrapedNumberInput
          placeholder={props.placeholder || props.title}
          result={props.result}
        />
      }
      newField={
        <ScrapedNumberInput
          placeholder={props.placeholder || props.title}
          result={props.result}
          isNew
          locked={props.locked}
          onChange={(value) =>
            props.onChange(props.result.cloneWithValue(value))
          }
        />
      }
      onChange={props.onChange}
    />
  );
};

interface IScrapedStringListProps {
  isNew?: boolean;
  placeholder?: string;
  locked?: boolean;
  result: ScrapeResult<string[]>;
  onChange?: (value: string[]) => void;
}

const ScrapedStringList: React.FC<IScrapedStringListProps> = (props) => {
  const value = props.isNew
    ? props.result.newValue
    : props.result.originalValue;

  return (
    <StringListInput
      value={value ?? []}
      setValue={(v) => {
        if (props.isNew && props.onChange) {
          props.onChange(v);
        }
      }}
      placeholder={props.placeholder}
      readOnly={!props.isNew || props.locked}
    />
  );
};

interface IScrapedStringListRowProps {
  mergeable?: boolean;
  title: string;
  field: string;
  placeholder?: string;
  result: ScrapeResult<string[]>;
  locked?: boolean;
  onChange: (value: ScrapeResult<string[]>) => void;
}

export const ScrapedStringListRow: React.FC<IScrapedStringListRowProps> = (
  props
) => {
  return (
    <ScrapeDialogRow
      className="string-list-row"
      title={props.title}
      field={props.field}
      result={props.result}
      mergeValues={
        props.field === "aliases" && props.mergeable !== false
          ? (original, scraped) =>
              mergeLists(original, scraped, (value) => value.toLowerCase())
          : undefined
      }
      mergedField={(value) => (
        <StringListInput value={value} setValue={() => {}} readOnly />
      )}
      originalField={
        <ScrapedStringList
          placeholder={props.placeholder || props.title}
          result={props.result}
        />
      }
      newField={
        <ScrapedStringList
          placeholder={props.placeholder || props.title}
          result={props.result}
          isNew
          locked={props.locked}
          onChange={(value) =>
            props.onChange(props.result.cloneWithValue(value))
          }
        />
      }
      onChange={props.onChange}
    />
  );
};

const ScrapedTextArea: React.FC<IScrapedInputGroupProps> = (props) => {
  return (
    <FormControl
      as="textarea"
      placeholder={props.placeholder}
      value={props.isNew ? props.result.newValue : props.result.originalValue}
      readOnly={!props.isNew}
      onChange={(e) => {
        if (props.isNew && props.onChange) {
          props.onChange(e.target.value);
        }
      }}
      className="bg-secondary text-white border-secondary scene-description"
    />
  );
};

export const ScrapedTextAreaRow: React.FC<IScrapedInputGroupRowProps> = (
  props
) => {
  return (
    <ScrapeDialogRow
      title={props.title}
      field={props.field}
      result={props.result}
      mergeValues={props.mergeValues}
      mergedField={(value) => (
        <FormControl
          as="textarea"
          value={value}
          readOnly
          className="bg-secondary text-white border-secondary"
        />
      )}
      originalField={
        <ScrapedTextArea
          placeholder={props.placeholder || props.title}
          result={props.result}
        />
      }
      newField={
        <ScrapedTextArea
          placeholder={props.placeholder || props.title}
          result={props.result}
          isNew
          onChange={(value) =>
            props.onChange(props.result.cloneWithValue(value))
          }
        />
      }
      onChange={props.onChange}
    />
  );
};

interface IScrapedImageProps {
  isNew?: boolean;
  className?: string;
  placeholder?: string;
  result: ScrapeResult<string>;
}

const ScrapedImage: React.FC<IScrapedImageProps> = (props) => {
  const value = props.isNew
    ? props.result.newValue
    : props.result.originalValue;

  if (!value) {
    return null;
  }

  return (
    <img className={props.className} src={value} alt={props.placeholder} />
  );
};

interface IScrapedImageRowProps {
  title: string;
  field: string;
  className?: string;
  result: ScrapeResult<string>;
  onChange: (value: ScrapeResult<string>) => void;
}

export const ScrapedImageRow: React.FC<IScrapedImageRowProps> = (props) => {
  return (
    <ScrapeDialogRow
      title={props.title}
      field={props.field}
      result={props.result}
      originalField={
        <ScrapedImage
          result={props.result}
          className={props.className}
          placeholder={props.title}
        />
      }
      newField={
        <ScrapedImage
          result={props.result}
          className={props.className}
          placeholder={props.title}
          isNew
        />
      }
      onChange={props.onChange}
    />
  );
};

interface IScrapedImagesRowProps {
  title: string;
  field: string;
  className?: string;
  result: ScrapeResult<string>;
  images: string[];
  onChange: (value: ScrapeResult<string>) => void;
}

export const ScrapedImagesRow: React.FC<IScrapedImagesRowProps> = (props) => {
  const [imageIndex, setImageIndex] = useState(0);

  function onSetImageIndex(newIdx: number) {
    const ret = props.result.cloneWithValue(props.images[newIdx]);
    props.onChange(ret);
    setImageIndex(newIdx);
  }

  return (
    <ScrapeDialogRow
      title={props.title}
      field={props.field}
      result={props.result}
      originalField={
        <ScrapedImage
          result={props.result}
          className={props.className}
          placeholder={props.title}
        />
      }
      newField={
        <div className="image-selection-parent">
          <ImageSelector
            imageClassName={props.className}
            images={props.images}
            imageIndex={imageIndex}
            setImageIndex={onSetImageIndex}
          />
        </div>
      }
      onChange={props.onChange}
    />
  );
};

interface IScrapedCountryRowProps {
  title: string;
  field: string;
  result: ScrapeResult<string>;
  onChange: (value: ScrapeResult<string>) => void;
  locked?: boolean;
  locale?: string;
}

export const ScrapedCountryRow: React.FC<IScrapedCountryRowProps> = ({
  title,
  field,
  result,
  onChange,
  locked,
  locale,
}) => {
  const intl = useIntl();

  return (
    <ScrapeDialogRow
      title={title}
      field={field}
      result={result}
      originalField={
        <FormControl
          value={
            getCountryByISO(result.originalValue, locale, intl) ??
            result.originalValue
          }
          readOnly
          className="bg-secondary text-white border-secondary"
        />
      }
      newField={
        <CountrySelect
          value={result.newValue}
          disabled={locked}
          onChange={(value) => {
            if (onChange) {
              onChange(result.cloneWithValue(value));
            }
          }}
          showFlag={false}
          isClearable={false}
          className="flex-grow-1"
        />
      }
      onChange={onChange}
    />
  );
};

export const ScrapedCustomFieldRows: React.FC<{
  results: CustomFieldScrapeResults;
  onChange: (newCustomFields: CustomFieldScrapeResults) => void;
}> = ({ results, onChange }) => {
  return (
    <>
      {Array.from(results.entries()).map(([field, result]) => {
        const fieldName = `custom_${field}`;
        return (
          <ScrapedInputGroupRow
            className="custom-field"
            title={field}
            field={fieldName}
            key={fieldName}
            result={result}
            onChange={(newResult) => {
              const newResults = new Map(results);
              newResults.set(field, newResult);
              onChange(newResults);
            }}
          />
        );
      })}
    </>
  );
};
