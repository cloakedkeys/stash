import React, { useEffect, useMemo, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Button, Form, Col, Row } from "react-bootstrap";
import Mousetrap from "mousetrap";
import * as yup from "yup";
import isEqual from "lodash-es/isEqual";
import { useFormik } from "formik";
import { Prompt } from "react-router-dom";
import * as GQL from "src/core/generated-graphql";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import {
  yupDateString,
  yupFormikValidate,
  yupUniqueStringList,
} from "src/utils/yup";
import { formikUtils } from "src/utils/form";
import { useTagsEdit } from "src/hooks/tagsEdit";

interface IProps {
  story: GQL.StoryDataFragment;
  isVisible: boolean;
  onSubmit: (input: Omit<GQL.StoryUpdateInput, "id">) => Promise<void>;
  onDelete?: () => void;
}

export const StoryEditPanel: React.FC<IProps> = ({
  story,
  isVisible,
  onSubmit,
  onDelete,
}) => {
  const intl = useIntl();
  const Toast = useToast();
  const [isLoading, setIsLoading] = useState(false);

  const schema = yup.object({
    title: yup.string().ensure(),
    author: yup.string().ensure(),
    source_website: yup.string().ensure(),
    urls: yupUniqueStringList(intl),
    date: yupDateString(intl),
    language: yup.string().ensure(),
    tagline: yup.string().ensure(),
    details: yup.string().ensure(),
    tag_ids: yup.array(yup.string().required()).defined(),
  });

  type InputValues = yup.InferType<typeof schema>;

  const initialValues = useMemo(
    () => ({
      title: story.title ?? "",
      author: story.author ?? "",
      source_website: story.source_website ?? "",
      urls: story.urls ?? [],
      date: story.date ?? "",
      language: story.language ?? "",
      tagline: story.tagline ?? "",
      details: story.details ?? "",
      tag_ids: story.tags.map((t) => t.id),
    }),
    [story]
  );

  async function onSave(input: InputValues) {
    setIsLoading(true);
    try {
      await onSubmit(input);
      formik.resetForm();
    } catch (e) {
      Toast.error(e);
    }
    setIsLoading(false);
  }

  const formik = useFormik<InputValues>({
    initialValues,
    enableReinitialize: true,
    validate: yupFormikValidate(schema),
    onSubmit: (values) => onSave(schema.cast(values)),
  });

  const { tagsControl } = useTagsEdit(story.tags, (ids) =>
    formik.setFieldValue("tag_ids", ids)
  );

  useEffect(() => {
    if (isVisible) {
      Mousetrap.bind("s s", () => {
        if (formik.dirty) {
          formik.submitForm();
        }
      });
      Mousetrap.bind("d d", () => {
        if (onDelete) {
          onDelete();
        }
      });

      return () => {
        Mousetrap.unbind("s s");
        Mousetrap.unbind("d d");
      };
    }
  });

  if (isLoading) return <LoadingIndicator />;

  const splitProps = {
    labelProps: { column: true, sm: 3 },
    fieldProps: { sm: 9 },
  };
  const fullWidthProps = {
    labelProps: { column: true, sm: 3, xl: 12 },
    fieldProps: { sm: 9, xl: 12 },
  };
  const { renderField, renderInputField, renderDateField, renderURLListField } =
    formikUtils(intl, formik, splitProps);

  return (
    <div id="story-edit-details">
      <Prompt
        when={formik.dirty}
        message={intl.formatMessage({ id: "dialogs.unsaved_changes" })}
      />

      <Form noValidate onSubmit={formik.handleSubmit}>
        <Row className="form-container edit-buttons-container px-3 pt-3">
          <div className="edit-buttons mb-3 pl-0">
            <Button
              className="edit-button"
              variant="primary"
              disabled={!formik.dirty || !isEqual(formik.errors, {})}
              onClick={() => formik.submitForm()}
            >
              <FormattedMessage id="actions.save" />
            </Button>
            {onDelete && (
              <Button
                className="edit-button"
                variant="danger"
                onClick={() => onDelete()}
              >
                <FormattedMessage id="actions.delete" />
              </Button>
            )}
          </div>
        </Row>
        <Row className="form-container px-3">
          <Col xl={12}>
            {renderInputField("title")}
            {renderInputField("author")}
            {renderInputField("source_website")}
            {renderURLListField("urls", undefined, undefined, "urls", {
              labelProps: { column: true, md: 3, lg: 12 },
              fieldProps: { md: 9, lg: 12 },
            })}
            {renderDateField("date")}
            {renderInputField("language")}
            {renderField(
              "tag_ids",
              intl.formatMessage({ id: "tags" }),
              tagsControl(),
              fullWidthProps
            )}
            {renderInputField("tagline")}
            {renderInputField("details", "textarea", "details", {
              labelProps: { column: true, sm: 3, lg: 12 },
              fieldProps: { sm: 9, lg: 12 },
            })}
          </Col>
        </Row>
      </Form>
    </div>
  );
};

export default StoryEditPanel;
