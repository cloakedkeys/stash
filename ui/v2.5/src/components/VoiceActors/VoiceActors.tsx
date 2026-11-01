import { useState } from "react";
import { Link, Route, Switch, useHistory, useParams } from "react-router-dom";
import { Alert, Button, Form, Table } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { FavoriteIcon } from "../Shared/FavoriteIcon";
import { ModifyRoute } from "../Shared/ModifyRoute";
import { useCanModify } from "src/hooks/CurrentUser";

const heading = (
  <FormattedMessage id="voice_actors" defaultMessage="Voice actors" />
);

function VoiceActorList() {
  const canModify = useCanModify();
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [favorite, setFavorite] = useState("");
  const { data, loading, error } = GQL.useFindVoiceActorsQuery({
    variables: {
      filter: { q, page, per_page: 40 },
      favorite: favorite === "" ? undefined : favorite === "true",
    },
    fetchPolicy: "cache-and-network",
  });
  return (
    <section className="mt-3">
      <div className="d-flex justify-content-between align-items-center mb-3">
        <h2>{heading}</h2>
        {canModify && (
          <Link className="btn btn-primary" to="/voice-actors/new">
            <FormattedMessage id="actions.create" defaultMessage="Create" />
          </Link>
        )}
      </div>
      <Form.Control
        aria-label="Search voice actors"
        placeholder="Search names or aliases"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setPage(1);
        }}
      />
      <Form.Control
        as="select"
        className="my-3"
        aria-label="Filter by favorite"
        value={favorite}
        onChange={(e) => {
          setFavorite(e.target.value);
          setPage(1);
        }}
      >
        <option value="">All voice actors</option>
        <option value="true">Favorites only</option>
        <option value="false">Non-favorites only</option>
      </Form.Control>
      {error && <Alert variant="danger">{error.message}</Alert>}
      {loading && !data && <LoadingIndicator />}
      <p>{data?.findVoiceActors.count ?? 0} voice actors</p>
      <Table responsive>
        <thead>
          <tr>
            <th>
              <FormattedMessage id="name" />
            </th>
            <th>
              <FormattedMessage id="aliases" defaultMessage="Aliases" />
            </th>
            <th>
              <FormattedMessage id="scenes" />
            </th>
          </tr>
        </thead>
        <tbody>
          {data?.findVoiceActors.voice_actors.map((actor) => (
            <tr key={actor.id}>
              <td>
                <Link to={`/voice-actors/${actor.id}`}>
                  {actor.favorite ? "★ " : ""}
                  {actor.name}
                </Link>
              </td>
              <td>{actor.aliases.join(", ")}</td>
              <td>{actor.scene_count}</td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Button disabled={page === 1} onClick={() => setPage(page - 1)}>
        Previous
      </Button>{" "}
      <span>{page}</span>{" "}
      <Button
        disabled={page * 40 >= (data?.findVoiceActors.count ?? 0)}
        onClick={() => setPage(page + 1)}
      >
        Next
      </Button>
    </section>
  );
}

function VoiceActorForm({
  actor,
  onSave,
  saving,
}: {
  actor?: GQL.VoiceActorDataFragment;
  onSave: (input: GQL.VoiceActorCreateInput) => Promise<void>;
  saving: boolean;
}) {
  const [name, setName] = useState(actor?.name ?? "");
  const [details, setDetails] = useState(actor?.details ?? "");
  const [aliases, setAliases] = useState(actor?.aliases.join("\n") ?? "");
  const [urls, setURLs] = useState(actor?.urls.join("\n") ?? "");
  const [favorite, setFavorite] = useState(actor?.favorite ?? false);
  const [error, setError] = useState("");
  const lines = (text: string) =>
    text
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
  return (
    <Form
      onSubmit={async (e) => {
        e.preventDefault();
        setError("");
        try {
          await onSave({
            name: name.trim(),
            details,
            aliases: lines(aliases),
            urls: lines(urls),
            favorite,
          });
        } catch (err) {
          setError(err instanceof Error ? err.message : String(err));
        }
      }}
    >
      {error && <Alert variant="danger">{error}</Alert>}
      <Form.Group controlId="voice-actor-name">
        <Form.Label>
          <FormattedMessage id="name" />
        </Form.Label>
        <Form.Control
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Form.Group>
      <Form.Group controlId="voice-actor-details">
        <Form.Label>
          <FormattedMessage id="details" />
        </Form.Label>
        <Form.Control
          as="textarea"
          rows={5}
          value={details}
          onChange={(e) => setDetails(e.target.value)}
        />
      </Form.Group>
      <Form.Group controlId="voice-actor-aliases">
        <Form.Label>Aliases (one per line)</Form.Label>
        <Form.Control
          as="textarea"
          value={aliases}
          onChange={(e) => setAliases(e.target.value)}
        />
      </Form.Group>
      <Form.Group controlId="voice-actor-urls">
        <Form.Label>URLs (one per line)</Form.Label>
        <Form.Control
          as="textarea"
          value={urls}
          onChange={(e) => setURLs(e.target.value)}
        />
      </Form.Group>
      <Form.Check
        id="voice-actor-favorite"
        className="mb-3"
        label="Favorite"
        checked={favorite}
        onChange={(e) => setFavorite(e.target.checked)}
      />
      <Button type="submit" disabled={saving || !name.trim()}>
        <FormattedMessage id="actions.save" defaultMessage="Save" />
      </Button>{" "}
      <Link
        className="btn btn-secondary"
        to={actor ? `/voice-actors/${actor.id}` : "/voice-actors"}
      >
        <FormattedMessage id="actions.cancel" defaultMessage="Cancel" />
      </Link>
    </Form>
  );
}

function VoiceActorDetail({
  editing = false,
  creating = false,
}: {
  editing?: boolean;
  creating?: boolean;
}) {
  const { id } = useParams<{ id: string }>();
  const history = useHistory();
  const { data, loading, error, refetch } = GQL.useFindVoiceActorQuery({
    variables: { id },
    skip: creating,
    fetchPolicy: "cache-and-network",
  });
  const canModify = useCanModify();
  const [setUserData] = GQL.useVoiceActorUserDataUpdateMutation();
  const [create, creatingNow] = GQL.useVoiceActorCreateMutation();
  const [update, updating] = GQL.useVoiceActorUpdateMutation();
  const [destroy, destroying] = GQL.useVoiceActorDestroyMutation();
  const [deleteError, setDeleteError] = useState("");
  if (loading && !data && !creating) return <LoadingIndicator />;
  if (error && !creating)
    return <Alert variant="danger">{error.message}</Alert>;
  const actor = creating ? undefined : data?.findVoiceActor;
  if (!creating && !actor)
    return <Alert variant="warning">Voice actor not found</Alert>;
  return (
    <section className="mt-3">
      <Link to="/voice-actors">{heading}</Link>
      <h2 className="my-3">{creating ? "Create voice actor" : actor?.name}</h2>
      {editing || creating ? (
        <VoiceActorForm
          actor={actor ?? undefined}
          key={actor?.id ?? "new"}
          saving={creatingNow.loading || updating.loading}
          onSave={async (input) => {
            if (creating) {
              const result = await create({ variables: { input } });
              history.replace(
                `/voice-actors/${result.data!.voiceActorCreate.id}`
              );
            } else {
              await update({ variables: { input: { id, ...input } } });
              await refetch();
              history.push(`/voice-actors/${id}`);
            }
          }}
        />
      ) : (
        actor && (
          <>
            <div className="mb-3">
              <FavoriteIcon
                favorite={actor.favorite}
                onToggleFavorite={(favorite) =>
                  setUserData({ variables: { input: { id, favorite } } })
                }
              />
            </div>
            {canModify && (
              <div className="mb-3">
                <Link
                  className="btn btn-primary"
                  to={`/voice-actors/${id}/edit`}
                >
                  <FormattedMessage id="actions.edit" defaultMessage="Edit" />
                </Link>{" "}
                <Button
                  variant="danger"
                  disabled={destroying.loading}
                  onClick={async () => {
                    if (
                      !window.confirm(
                        `Delete voice actor ${actor.name}? Scene voice credits will be removed.`
                      )
                    )
                      return;
                    try {
                      await destroy({ variables: { id } });
                      history.push("/voice-actors");
                    } catch (err) {
                      setDeleteError(
                        err instanceof Error ? err.message : String(err)
                      );
                    }
                  }}
                >
                  <FormattedMessage
                    id="actions.delete"
                    defaultMessage="Delete"
                  />
                </Button>
              </div>
            )}
            {deleteError && <Alert variant="danger">{deleteError}</Alert>}
            {actor.aliases.length > 0 && (
              <p>
                <strong>Aliases:</strong> {actor.aliases.join(", ")}
              </p>
            )}
            <p className="pre">{actor.details}</p>
            <ul>
              {actor.urls.map((url) => (
                <li key={url}>
                  <a
                    href={/^https?:\/\//i.test(url) ? url : undefined}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {url}
                  </a>
                </li>
              ))}
            </ul>
            <h4>
              <FormattedMessage id="scenes" /> ({actor.scene_count})
            </h4>
            <div className="row">
              {actor.scenes.map((scene) => (
                <div className="col-sm-6 col-lg-3 mb-3" key={scene.id}>
                  <Link to={`/scenes/${scene.id}`}>
                    {scene.paths.screenshot && (
                      <img
                        className="img-fluid"
                        src={scene.paths.screenshot}
                        alt=""
                      />
                    )}
                    <p>
                      {scene.title ||
                        scene.files[0]?.basename ||
                        `Scene ${scene.id}`}
                    </p>
                  </Link>
                </div>
              ))}
            </div>
          </>
        )
      )}
    </section>
  );
}

export default function VoiceActors() {
  return (
    <Switch>
      <ModifyRoute
        exact
        path="/voice-actors/new"
        render={() => <VoiceActorDetail creating />}
      />
      <ModifyRoute
        exact
        path="/voice-actors/:id/edit"
        render={() => <VoiceActorDetail editing />}
      />
      <Route exact path="/voice-actors/:id" component={VoiceActorDetail} />
      <Route component={VoiceActorList} />
    </Switch>
  );
}
