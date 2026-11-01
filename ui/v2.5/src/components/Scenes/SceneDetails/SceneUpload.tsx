import React, { useRef, useState } from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { Link } from "react-router-dom";
import { getClient } from "src/core/StashService";
import * as GQL from "src/core/generated-graphql";
import { useConfigurationContext } from "src/hooks/Config";

interface UploadResult {
  name: string;
  status: "pending" | "uploading" | "saved" | "failed";
  path?: string;
  sceneIDs?: string[];
  error?: string;
}

export const SceneUpload: React.FC = () => {
  const intl = useIntl();
  const { configuration } = useConfigurationContext();
  const { stashes, videoExtensions, maxUploadSize } = configuration.general;
  const libraries = stashes.filter((stash) => !stash.excludeVideo);
  const [library, setLibrary] = useState(libraries[0]?.path ?? "");
  const [files, setFiles] = useState<File[]>([]);
  const [results, setResults] = useState<UploadResult[]>([]);
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const limitMessage = intl.formatMessage(
    { id: "scene_upload.limit" },
    { limit: intl.formatNumber(maxUploadSize / 1048576) }
  );

  async function upload() {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    const batch: UploadResult[] = files.map((file) => ({
      name: file.name,
      status: "pending",
    }));
    const update = () => setResults(batch.map((result) => ({ ...result })));
    update();
    try {
      for (const [index, file] of files.entries()) {
        const result = batch[index];
        result.status = "uploading";
        update();
        try {
          if (file.size >= maxUploadSize) {
            throw new Error(
              intl.formatMessage({ id: "scene_upload.too_large" })
            );
          }
          const response = await getClient().mutate<
            GQL.SceneUploadMutation,
            GQL.SceneUploadMutationVariables
          >({
            mutation: GQL.SceneUploadDocument,
            variables: { file, library_path: library },
          });
          const saved = response.data?.sceneUpload;
          if (!saved) {
            throw new Error(
              intl.formatMessage({ id: "scene_upload.no_result" })
            );
          }
          result.status = "saved";
          result.path = saved.path;
          result.sceneIDs = saved.scenes.map((scene) => scene.id);
        } catch (error) {
          result.status = "failed";
          result.error = error instanceof Error ? error.message : String(error);
        }
        update();
      }
    } finally {
      running.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="mb-4" aria-labelledby="scene-upload-title">
      <h3 id="scene-upload-title">
        <FormattedMessage id="scene_upload.title" />
      </h3>
      <p>{limitMessage}</p>
      {libraries.length === 0 ? (
        <p>
          <FormattedMessage id="scene_upload.no_libraries" />
        </p>
      ) : (
        <>
          <Form.Group controlId="scene-upload-library">
            <Form.Label>
              <FormattedMessage id="scene_upload.library" />
            </Form.Label>
            <Form.Control
              as="select"
              value={library}
              disabled={busy}
              onChange={(event) => setLibrary(event.currentTarget.value)}
            >
              {libraries.map((stash) => (
                <option key={stash.path} value={stash.path}>
                  {stash.path}
                </option>
              ))}
            </Form.Control>
          </Form.Group>
          <Form.Group controlId="scene-upload-files">
            <Form.Label>
              <FormattedMessage id="scene_upload.files" />
            </Form.Label>
            <Form.Control
              type="file"
              multiple
              accept={videoExtensions
                .map((ext) => `.${ext.replace(/^\./, "")}`)
                .join(",")}
              disabled={busy}
              onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                setFiles(Array.from(event.currentTarget.files ?? []));
                setResults([]);
              }}
            />
          </Form.Group>
          <Button disabled={busy || !files.length || !library} onClick={upload}>
            <FormattedMessage
              id={busy ? "scene_upload.uploading" : "scene_upload.start"}
            />
          </Button>
        </>
      )}
      <ul aria-live="polite" aria-busy={busy}>
        {results.map((result, index) => (
          <li key={`${index}-${result.name}`}>
            {result.name}:{" "}
            <FormattedMessage id={`scene_upload.${result.status}`} />
            {result.path && <> — {result.path}</>}
            {result.error && <> — {result.error}</>}
            {result.sceneIDs?.map((id) => (
              <React.Fragment key={id}>
                {" "}
                <Link to={`/scenes/${id}`}>
                  <FormattedMessage id="scene_upload.view_scene" />
                </Link>
              </React.Fragment>
            ))}
          </li>
        ))}
      </ul>
    </section>
  );
};
