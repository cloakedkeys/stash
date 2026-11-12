import React, { useEffect, useMemo, useRef, useState } from "react";
import * as GQL from "src/core/generated-graphql";
import { objectTitle } from "src/core/files";
import {
  useFindScene,
  useGroupCreate,
  useSceneUpdate,
} from "src/core/StashService";
import { useHistory, useLocation } from "react-router-dom";
import { useIntl } from "react-intl";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import ImageUtils from "src/utils/image";
import { GroupEditPanel } from "./GroupEditPanel";

const GroupCreate: React.FC = () => {
  const history = useHistory();
  const intl = useIntl();
  const Toast = useToast();

  const location = useLocation();
  const query = useMemo(() => new URLSearchParams(location.search), [location]);
  const sourceSceneID = query.get("from_scene_id");
  const { data, loading } = useFindScene(sourceSceneID ?? "new");
  const scene = sourceSceneID ? data?.findScene : undefined;
  const [sourceImage, setSourceImage] = useState<string>();
  const [loadingImage, setLoadingImage] = useState(false);
  // A failed scene update can be retried without creating another group.
  const createdGroupID = useRef<string>();

  useEffect(() => {
    let cancelled = false;
    setSourceImage(undefined);
    const screenshot = scene?.paths.screenshot;
    setLoadingImage(!!screenshot);
    if (!screenshot) return;
    ImageUtils.imageToDataURL(screenshot)
      .then((image) => {
        if (!cancelled) setSourceImage(image);
      })
      .catch((error) => Toast.error(error))
      .finally(() => {
        if (!cancelled) setLoadingImage(false);
      });
    return () => {
      cancelled = true;
    };
  }, [scene?.paths.screenshot, Toast]);

  const group = useMemo(
    () =>
      scene
        ? {
            name: objectTitle(scene),
            date: scene.date,
            studio: scene.studio,
            director: scene.director,
            urls: scene.urls,
            front_image: sourceImage,
          }
        : {
            name: query.get("q") ?? undefined,
          },
    [scene, sourceImage, query]
  );

  // Editing group state
  const [frontImage, setFrontImage] = useState<string | null>();
  const [backImage, setBackImage] = useState<string | null>();
  const [encodingImage, setEncodingImage] = useState<boolean>(false);

  const [createGroup] = useGroupCreate();
  const [updateScene] = useSceneUpdate();

  async function onSave(input: GQL.GroupCreateInput, andNew?: boolean) {
    const groupID =
      createdGroupID.current ??
      (
        await createGroup({
          variables: { input },
        })
      ).data?.groupCreate?.id;
    if (groupID) {
      createdGroupID.current = groupID;
      if (scene) {
        await updateScene({
          variables: {
            input: {
              id: scene.id,
              groups: [
                ...scene.groups.map((entry) => ({
                  group_id: entry.group.id,
                  scene_index: entry.scene_index,
                })),
                { group_id: groupID, scene_index: 1 },
              ],
            },
          },
        });
      }
      createdGroupID.current = undefined;
      if (!andNew) {
        history.push(`/groups/${groupID}`);
      } else if (sourceSceneID) {
        history.replace("/groups/new");
      }
      Toast.success(
        intl.formatMessage(
          { id: "toast.created_entity" },
          { entity: intl.formatMessage({ id: "group" }).toLocaleLowerCase() }
        )
      );
    }
  }

  function renderFrontImage() {
    if (frontImage) {
      return (
        <div className="group-image-container">
          <img alt="Front Cover" src={frontImage} />
        </div>
      );
    }
  }

  function renderBackImage() {
    if (backImage) {
      return (
        <div className="group-image-container">
          <img alt="Back Cover" src={backImage} />
        </div>
      );
    }
  }

  if (loading || loadingImage) return <LoadingIndicator />;

  // TODO: CSS class
  return (
    <div className="row">
      <div className="group-details mb-3 col">
        <div className="logo w-100">
          {encodingImage ? (
            <LoadingIndicator
              message={intl.formatMessage({ id: "actions.encoding_image" })}
            />
          ) : (
            <div className="group-images">
              {renderFrontImage()}
              {renderBackImage()}
            </div>
          )}
        </div>

        <GroupEditPanel
          group={group}
          onSubmit={onSave}
          onCancel={() => history.push("/groups")}
          onDelete={() => {}}
          setFrontImage={setFrontImage}
          setBackImage={setBackImage}
          setEncodingImage={setEncodingImage}
        />
      </div>
    </div>
  );
};

export default GroupCreate;
