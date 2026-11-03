import React, { useState } from "react";
import { Form } from "react-bootstrap";
import { useIntl } from "react-intl";
import { Link, useHistory } from "react-router-dom";
import { faDownload } from "@fortawesome/free-solid-svg-icons";
import {
  mutateStoryCreate,
  queryFindStoriesByURL,
  queryScrapeStoryURL,
  useListStoryScrapers,
} from "src/core/StashService";
import { useToast } from "src/hooks/Toast";
import { ModalComponent } from "../Shared/Modal";
import { storyURLScrapable } from "./StoryDetails/storyScrape";
import { IStoryWithURLs, importStory } from "./storyImport";

interface IProblem {
  message: string;
  existing?: IStoryWithURLs;
}

// Adds a story to Stories from its web address, with the installed scraper
// for that site (#1964).
export const ImportStoryDialog: React.FC<{ onClose: () => void }> = ({
  onClose,
}) => {
  const intl = useIntl();
  const Toast = useToast();
  const history = useHistory();
  const scrapers = useListStoryScrapers();

  const [url, setURL] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<IProblem>();

  function message(id: string) {
    return intl.formatMessage({ id: `dialogs.import_story.${id}` });
  }

  async function onImport() {
    const address = url.trim();
    if (!address || busy) return;
    setProblem(undefined);
    setBusy(true);
    try {
      const result = await importStory(address, {
        scrapable: (u) => storyURLScrapable(scrapers.data?.listScrapers, u),
        findByURL: async (u) =>
          (await queryFindStoriesByURL(u)).data.findStories.stories,
        scrape: async (u) =>
          (await queryScrapeStoryURL(u)).data?.scrapeStoryURL,
        create: async (input) => {
          const id = (await mutateStoryCreate(input)).data?.storyCreate?.id;
          if (!id) throw new Error("the story wasn't added");
          return id;
        },
      });

      switch (result.kind) {
        case "created":
          Toast.success(
            intl.formatMessage(
              { id: "toast.created_entity" },
              { entity: intl.formatMessage({ id: "story" }) }
            )
          );
          onClose();
          history.push(`/stories/${result.id}`);
          return;
        case "exists":
          setProblem({
            message: message("already_added"),
            existing: result.story,
          });
          return;
        default:
          setProblem({ message: message(result.kind) });
      }
    } catch (e) {
      Toast.error(e);
    } finally {
      setBusy(false);
    }
  }

  return (
    <ModalComponent
      show
      icon={faDownload}
      header={message("title")}
      dialogClassName="import-story-dialog"
      onHide={onClose}
      isRunning={busy}
      disabled={!url.trim() || scrapers.loading}
      accept={{
        text: intl.formatMessage({ id: "actions.add" }),
        onClick: () => onImport(),
      }}
      cancel={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: onClose,
      }}
    >
      <Form
        onSubmit={(e) => {
          e.preventDefault();
          onImport();
        }}
      >
        <Form.Group controlId="import-story-url">
          <Form.Label>{message("address")}</Form.Label>
          <Form.Control
            className="text-input"
            type="url"
            inputMode="url"
            autoFocus
            placeholder="https://"
            value={url}
            onChange={(e) => {
              setURL(e.currentTarget.value);
              setProblem(undefined);
            }}
          />
          <Form.Text className="text-muted">{message("help")}</Form.Text>
        </Form.Group>
      </Form>
      {problem && (
        <p className="import-story-problem" role="alert">
          {problem.message}
          {problem.existing && (
            <>
              {" "}
              <Link to={`/stories/${problem.existing.id}`} onClick={onClose}>
                {problem.existing.title || url.trim()}
              </Link>
            </>
          )}
        </p>
      )}
    </ModalComponent>
  );
};
