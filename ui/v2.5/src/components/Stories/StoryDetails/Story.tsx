import React, { useEffect, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Nav, Tab } from "react-bootstrap";
import { Helmet } from "react-helmet";
import { Link, useHistory, useParams } from "react-router-dom";
import * as GQL from "src/core/generated-graphql";
import {
  useFindStory,
  useStoriesDestroy,
  useStoryUpdate,
  useStoryUserDataUpdate,
} from "src/core/StashService";
import { objectTitle } from "src/core/files";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { ErrorMessage } from "src/components/Shared/ErrorMessage";
import { RatingSystem } from "src/components/Shared/Rating/RatingSystem";
import { DeleteEntityDialog } from "src/components/Shared/DeleteEntityDialog";
import { FileSize } from "src/components/Shared/FileSize";
import { useCanModify } from "src/hooks/CurrentUser";
import { useToast } from "src/hooks/Toast";
import { StoryEditPanel } from "./StoryEditPanel";

// StoryText shows the story's text. The server sends it as plain UTF-8, and
// it's shown as text, so markup in a story file is never run.
export const StoryText: React.FC<{ url: string }> = ({ url }) => {
  const [text, setText] = useState<string>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    let cancelled = false;
    setText(undefined);
    setError(undefined);

    fetch(url)
      .then(async (res) => {
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
        const body = await res.text();
        if (!cancelled) setText(body);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      });

    return () => {
      cancelled = true;
    };
  }, [url]);

  if (error) return <ErrorMessage error={error} />;
  if (text === undefined) return <LoadingIndicator />;

  return <div className="story-text">{text}</div>;
};

const StoryDetailsPanel: React.FC<{ story: GQL.StoryDataFragment }> = ({
  story,
}) => {
  function row(id: string, value: React.ReactNode) {
    if (!value) return null;
    return (
      <dl className="row" key={id}>
        <dt className="col-3 col-xl-2">
          <FormattedMessage id={id} />
        </dt>
        <dd className="col-9 col-xl-10">{value}</dd>
      </dl>
    );
  }

  return (
    <div className="story-details">
      {row("tagline", story.tagline)}
      {row("author", story.author)}
      {row("source_website", story.source_website)}
      {row("date", story.date)}
      {row("language", story.language)}
      {row(
        "urls",
        story.urls.length > 0 && (
          <ul className="newline-list">
            {story.urls.map((u) => (
              <li key={u}>
                <a href={u} target="_blank" rel="noopener noreferrer">
                  {u}
                </a>
              </li>
            ))}
          </ul>
        )
      )}
      {row(
        "tags",
        story.tags.length > 0 && (
          <ul className="comma-list">
            {story.tags.map((t) => (
              <li key={t.id}>
                <Link to={`/tags/${t.id}`}>{t.name}</Link>
              </li>
            ))}
          </ul>
        )
      )}
      {row("details", story.details && <p className="pre">{story.details}</p>)}
      {story.files.map((f) => (
        <React.Fragment key={f.id}>
          {row("path", f.path)}
          {row("filesize", <FileSize size={f.size} />)}
        </React.Fragment>
      ))}
    </div>
  );
};

const StoryPage: React.FC<{ story: GQL.StoryDataFragment }> = ({ story }) => {
  const intl = useIntl();
  const history = useHistory();
  const Toast = useToast();
  const canModify = useCanModify();

  const [updateStory] = useStoryUpdate();
  const [updateUserData] = useStoryUserDataUpdate();
  const title = objectTitle(story);
  const hasText = story.files.length > 0 && !!story.paths.text;

  const [activeTab, setActiveTab] = useState(
    hasText ? "story-read-panel" : "story-details-panel"
  );
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  async function onSave(input: Omit<GQL.StoryUpdateInput, "id">) {
    await updateStory({ variables: { input: { id: story.id, ...input } } });
    Toast.success(
      intl.formatMessage(
        { id: "toast.updated_entity" },
        { entity: intl.formatMessage({ id: "story" }).toLocaleLowerCase() }
      )
    );
  }

  function setRating(v: number | null) {
    updateUserData({ variables: { input: { id: story.id, rating100: v } } });
  }

  function onDeleteClosed(deleted: boolean) {
    setIsDeleteOpen(false);
    if (deleted) {
      history.push("/stories");
    }
  }

  return (
    <div className="row story-page">
      <Helmet>
        <title>{title}</title>
      </Helmet>

      {isDeleteOpen && (
        <DeleteEntityDialog
          selected={[{ id: story.id, name: title }]}
          onClose={onDeleteClosed}
          singularEntity={intl.formatMessage({ id: "story" })}
          pluralEntity={intl.formatMessage({ id: "stories" })}
          destroyMutation={useStoriesDestroy}
        />
      )}

      <div className="col-12 story-header">
        <h2>{title}</h2>
        {story.author && <h5 className="story-author">{story.author}</h5>}
        <RatingSystem
          value={story.rating100}
          onSetRating={(value) => setRating(value)}
          clickToRate
          withoutContext
        />
      </div>

      <div className="col-12">
        <Tab.Container
          activeKey={activeTab}
          onSelect={(k) => k && setActiveTab(k)}
        >
          <Nav variant="tabs" className="mr-auto">
            {hasText && (
              <Nav.Item>
                <Nav.Link eventKey="story-read-panel">
                  <FormattedMessage id="actions.read" />
                </Nav.Link>
              </Nav.Item>
            )}
            <Nav.Item>
              <Nav.Link eventKey="story-details-panel">
                <FormattedMessage id="details" />
              </Nav.Link>
            </Nav.Item>
            {canModify && (
              <Nav.Item>
                <Nav.Link eventKey="story-edit-panel">
                  <FormattedMessage id="actions.edit" />
                </Nav.Link>
              </Nav.Item>
            )}
          </Nav>

          <Tab.Content>
            {hasText && (
              <Tab.Pane eventKey="story-read-panel">
                {activeTab === "story-read-panel" && (
                  <StoryText url={story.paths.text!} />
                )}
              </Tab.Pane>
            )}
            <Tab.Pane eventKey="story-details-panel">
              <StoryDetailsPanel story={story} />
            </Tab.Pane>
            {canModify && (
              <Tab.Pane eventKey="story-edit-panel" mountOnEnter>
                <StoryEditPanel
                  story={story}
                  isVisible={activeTab === "story-edit-panel"}
                  onSubmit={onSave}
                  onDelete={() => setIsDeleteOpen(true)}
                />
              </Tab.Pane>
            )}
          </Tab.Content>
        </Tab.Container>
      </div>
    </div>
  );
};

const StoryLoader: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const { data, loading, error } = useFindStory(id);

  if (loading) return <LoadingIndicator />;
  if (error) return <ErrorMessage error={error.message} />;
  if (!data?.findStory) {
    return <ErrorMessage error={`No story found with id ${id}.`} />;
  }

  return <StoryPage story={data.findStory} />;
};

export default StoryLoader;
