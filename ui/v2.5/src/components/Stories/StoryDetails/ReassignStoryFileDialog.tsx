import React, { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { faSignOutAlt } from "@fortawesome/free-solid-svg-icons";
import { Col, Form, Row } from "react-bootstrap";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import * as FormUtils from "src/utils/form";
import { mutateStoryAssignFile } from "src/core/StashService";
import { Story, StorySelect } from "../StorySelect";

interface IReassignStoryFileDialogProps {
  storyID: string;
  file: { id: string; path: string };
  // the story keeps nothing once its only file goes, when it has no title
  removesStory: boolean;
  onClose: (moved?: Story) => void;
}

// Moves a story's text file onto another story, such as one scraped from a website.
export const ReassignStoryFileDialog: React.FC<
  IReassignStoryFileDialogProps
> = (props) => {
  const intl = useIntl();
  const Toast = useToast();
  const [stories, setStories] = useState<Story[]>([]);
  const [reassigning, setReassigning] = useState(false);

  const singularEntity = intl.formatMessage({ id: "file" });
  const pluralEntity = intl.formatMessage({ id: "files" });

  async function onAccept() {
    if (!stories.length) return;

    setReassigning(true);
    try {
      await mutateStoryAssignFile(props.storyID, stories[0].id, props.file.id);
      Toast.success(
        intl.formatMessage(
          { id: "toast.reassign_past_tense" },
          { count: 1, singularEntity, pluralEntity }
        )
      );
      props.onClose(stories[0]);
    } catch (e) {
      Toast.error(e);
      setReassigning(false);
    }
  }

  return (
    <ModalComponent
      show
      icon={faSignOutAlt}
      header={intl.formatMessage(
        { id: "dialogs.reassign_entity_title" },
        { count: 1, singularEntity, pluralEntity }
      )}
      accept={{
        onClick: onAccept,
        text: intl.formatMessage({ id: "actions.reassign" }),
      }}
      cancel={{
        onClick: () => props.onClose(),
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
      }}
      isRunning={reassigning}
    >
      <p className="text-break">{props.file.path}</p>
      <Form>
        <Form.Group controlId="dest" as={Row}>
          {FormUtils.renderLabel({
            title: intl.formatMessage({
              id: "dialogs.reassign_files.destination",
            }),
            labelProps: { column: true, sm: 3, xl: 12 },
          })}
          <Col sm={9} xl={12}>
            <StorySelect
              values={stories}
              onSelect={(items) => setStories(items)}
              excludeIds={[props.storyID]}
            />
          </Col>
        </Form.Group>
      </Form>
      <p>
        <FormattedMessage id="dialogs.reassign_story_file.text_replaced" />
      </p>
      {props.removesStory && (
        <p>
          <FormattedMessage id="dialogs.reassign_story_file.story_removed" />
        </p>
      )}
    </ModalComponent>
  );
};
