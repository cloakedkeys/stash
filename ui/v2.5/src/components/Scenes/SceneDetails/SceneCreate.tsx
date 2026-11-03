import React, { useEffect, useMemo, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { useHistory, useLocation } from "react-router-dom";
import { Form } from "react-bootstrap";
import { copySceneMarkers } from "./copySceneMarkers";
import { SceneEditPanel } from "./SceneEditPanel";
import * as GQL from "src/core/generated-graphql";
import {
  mutateCreateScene,
  useFindScene,
  useSceneMarkerCreate,
} from "src/core/StashService";
import ImageUtils from "src/utils/image";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import { PatchComponent } from "src/patch";
import { useCanAdminister } from "src/hooks/CurrentUser";
import { SceneUpload } from "./SceneUpload";

const SceneCreate: React.FC = PatchComponent("SceneCreate", () => {
  const history = useHistory();
  const intl = useIntl();
  const Toast = useToast();
  const isAdmin = useCanAdminister();
  const [createMarker] = useSceneMarkerCreate();
  const [copyMarkers, setCopyMarkers] = useState(false);

  const location = useLocation();
  const query = useMemo(() => new URLSearchParams(location.search), [location]);

  // create scene from provided scene id if applicable
  const { data, loading } = useFindScene(query.get("from_scene_id") ?? "new");
  const markers = query.has("from_scene_id")
    ? (data?.findScene?.scene_markers ?? [])
    : [];

  const [loadingCoverImage, setLoadingCoverImage] = useState(false);
  const [coverImage, setCoverImage] = useState<string>();

  const scene = useMemo(() => {
    if (data?.findScene) {
      return {
        ...data.findScene,
        paths: undefined,
        id: undefined,
      };
    }

    return {
      title: query.get("q") ?? undefined,
    };
  }, [data?.findScene, query]);

  useEffect(() => {
    async function fetchCoverImage() {
      const srcScene = data?.findScene;
      if (srcScene?.paths.screenshot) {
        setLoadingCoverImage(true);
        const imageData = await ImageUtils.imageToDataURL(
          srcScene.paths.screenshot
        );
        setCoverImage(imageData);
        setLoadingCoverImage(false);
      } else {
        setCoverImage(undefined);
      }
    }

    fetchCoverImage();
  }, [data?.findScene]);

  if (loading || loadingCoverImage) {
    return <LoadingIndicator />;
  }

  async function onSave(input: GQL.SceneCreateInput, andNew?: boolean) {
    const fileID = query.get("file_id") ?? undefined;
    const result = await mutateCreateScene({
      ...input,
      file_ids: fileID ? [fileID] : undefined,
    });
    if (result.data?.sceneCreate?.id) {
      const failed = await copySceneMarkers(
        markers,
        result.data.sceneCreate.id,
        async (variables) => {
          const copy = await createMarker({ variables });
          return !!copy.data?.sceneMarkerCreate && !copy.errors?.length;
        },
        copyMarkers
      );
      // a later scene made from this page starts unticked
      setCopyMarkers(false);
      if (!andNew || failed > 0) {
        history.push(`/scenes/${result.data.sceneCreate.id}`);
      }
      if (failed > 0) {
        Toast.toast({
          variant: "warning",
          content: intl.formatMessage(
            { id: "toast.scene_markers_copy_failed" },
            { count: failed }
          ),
        });
        return;
      }
      Toast.success(
        intl.formatMessage(
          { id: "toast.created_entity" },
          { entity: intl.formatMessage({ id: "scene" }).toLocaleLowerCase() }
        )
      );
    }
  }

  return (
    <div className="row new-view justify-content-center" id="create-scene-page">
      <div className="col-md-8">
        <h2>
          <FormattedMessage
            id="actions.create_entity"
            values={{ entityType: intl.formatMessage({ id: "scene" }) }}
          />
        </h2>
        {isAdmin && <SceneUpload />}
        {markers.length > 0 && (
          <Form.Group controlId="copy-scene-markers">
            <Form.Check
              type="checkbox"
              checked={copyMarkers}
              onChange={(event) => setCopyMarkers(event.currentTarget.checked)}
              aria-describedby="copy-scene-markers-note"
              label={intl.formatMessage(
                { id: "split_scene.copy_markers" },
                { count: markers.length }
              )}
            />
            <Form.Text id="copy-scene-markers-note" className="text-muted">
              <FormattedMessage id="split_scene.copy_markers_note" />
            </Form.Text>
          </Form.Group>
        )}
        <SceneEditPanel
          scene={scene}
          initialCoverImage={coverImage}
          isVisible
          isNew
          onSubmit={onSave}
        />
      </div>
    </div>
  );
});

export default SceneCreate;
