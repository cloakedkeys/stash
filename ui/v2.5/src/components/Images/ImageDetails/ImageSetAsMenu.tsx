import React, { useRef, useState } from "react";
import { Button, Dropdown } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { Link } from "react-router-dom";
import * as GQL from "src/core/generated-graphql";
import { usePerformerUpdate, useTagUpdate } from "src/core/StashService";
import { PerformerSelect } from "src/components/Performers/PerformerSelect";
import { TagSelect } from "src/components/Tags/TagSelect";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import { imageToPictureDataURL, isImageClip } from "src/utils/imagePicture";

type Target = { id: string; name: string; disambiguation?: string | null };
type TargetType = "performer" | "tag";

export const ImageSetAsMenu: React.FC<{ image: GQL.ImageDataFragment }> = ({
  image,
}) => {
  const intl = useIntl();
  const Toast = useToast();
  const [updatePerformer] = usePerformerUpdate();
  const [updateTag] = useTagUpdate();
  const [targetType, setTargetType] = useState<TargetType>();
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);

  function close() {
    if (!busy.current) setTargetType(undefined);
  }

  async function save(target: Target) {
    if (busy.current || !targetType) return;
    busy.current = true;
    setSaving(true);
    try {
      const imageData = await imageToPictureDataURL(image, intl);
      const update = targetType === "performer" ? updatePerformer : updateTag;
      await update({
        variables: { input: { id: target.id, image: imageData } },
      });
      setTargetType(undefined);
      Toast.success(
        <>
          <FormattedMessage
            id="toast.updated_entity"
            values={{ entity: intl.formatMessage({ id: targetType }) }}
          />{" "}
          <Link
            to={`/${targetType === "performer" ? "performers" : "tags"}/${target.id}`}
          >
            {target.name}
          </Link>
        </>
      );
    } catch (e) {
      Toast.error(e);
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }

  if (isImageClip(image)) return null;

  const linkedTargets =
    targetType === "performer" ? image.performers : image.tags;
  return (
    <>
      {(["performer", "tag"] as const).map((type) => (
        <Dropdown.Item
          key={type}
          className="bg-secondary text-white"
          onClick={() => setTargetType(type)}
        >
          <FormattedMessage id={`actions.set_as_${type}_image`} />…
        </Dropdown.Item>
      ))}
      {targetType && (
        <ModalComponent
          show
          header={intl.formatMessage({
            id: `actions.set_as_${targetType}_image`,
          })}
          onHide={close}
          accept={{
            text: intl.formatMessage({ id: "actions.cancel" }),
            variant: "secondary",
            onClick: close,
          }}
          isRunning={saving}
          escapeAfterChanges
        >
          {linkedTargets.map((target: Target) => (
            <Button
              key={target.id}
              className="mr-2 mb-2"
              disabled={saving}
              onClick={() => save(target)}
            >
              {target.name}
              {target.disambiguation ? ` (${target.disambiguation})` : ""}
            </Button>
          ))}
          {targetType === "performer" ? (
            <PerformerSelect
              creatable={false}
              isDisabled={saving}
              onSelect={(targets) => {
                if (targets[0]) void save(targets[0]);
              }}
            />
          ) : (
            <TagSelect
              creatable={false}
              isDisabled={saving}
              onSelect={(targets) => {
                if (targets[0]) void save(targets[0]);
              }}
            />
          )}
        </ModalComponent>
      )}
    </>
  );
};
