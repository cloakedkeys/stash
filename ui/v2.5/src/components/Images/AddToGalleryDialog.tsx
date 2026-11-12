import React, { useEffect, useRef, useState } from "react";
import { Button } from "react-bootstrap";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";
import {
  queryFindGalleriesByIDForSelect,
  useBulkImageUpdate,
  useConfigureUISetting,
} from "src/core/StashService";
import { BulkUpdateIdMode } from "src/core/generated-graphql";
import { galleryTitle } from "src/core/galleries";
import { useConfigurationContext } from "src/hooks/Config";
import { useToast } from "src/hooks/Toast";
import { Gallery, GallerySelect } from "../Galleries/GallerySelect";
import { ModalComponent } from "../Shared/Modal";

export const AddToGalleryDialog: React.FC<{
  imageIds: string[];
  onClose: (applied: boolean) => void;
}> = ({ imageIds, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const { configuration } = useConfigurationContext();
  const recentIds = configuration.ui.recentImageGalleries;
  const [recentGalleries, setRecentGalleries] = useState<Gallery[]>([]);
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);
  const [updateImages] = useBulkImageUpdate();
  const [saveUISetting] = useConfigureUISetting();

  useEffect(() => {
    let cancelled = false;
    setRecentGalleries([]);
    if (recentIds?.length) {
      queryFindGalleriesByIDForSelect(recentIds.slice(0, 5))
        .then(({ data }) => {
          if (cancelled) return;
          const galleries = data.findGalleries.galleries;
          setRecentGalleries(
            recentIds.slice(0, 5).flatMap((id) => {
              const gallery = galleries.find((g) => g.id === id);
              return gallery ? [gallery] : [];
            })
          );
        })
        .catch((e) => {
          if (!cancelled) Toast.error(e);
        });
    }
    return () => {
      cancelled = true;
    };
  }, [recentIds, Toast]);

  async function addToGallery(gallery: Gallery) {
    if (busy.current || !imageIds.length) return;
    busy.current = true;
    setSaving(true);
    try {
      await updateImages({
        variables: {
          input: {
            ids: imageIds,
            gallery_ids: { ids: [gallery.id], mode: BulkUpdateIdMode.Add },
          },
        },
      });
    } catch (e) {
      Toast.error(e);
      busy.current = false;
      setSaving(false);
      return;
    }

    Toast.success(
      <>
        {intl.formatMessage(
          { id: "toast.images_added_to_gallery" },
          { count: imageIds.length }
        )}{" "}
        <Link to={`/galleries/${gallery.id}`}>{galleryTitle(gallery)}</Link>
      </>
    );
    try {
      await saveUISetting({
        variables: {
          key: "recentImageGalleries",
          value: [...new Set([gallery.id, ...(recentIds ?? [])])].slice(0, 5),
        },
      });
    } catch (e) {
      Toast.error(e);
    }
    onClose(true);
  }

  return (
    <ModalComponent
      show
      header={intl.formatMessage({ id: "actions.add_to_gallery" })}
      cancel={{ onClick: () => onClose(false) }}
      isRunning={saving}
      escapeAfterChanges
    >
      {recentGalleries.length > 0 && (
        <div className="mb-3">
          <p>{intl.formatMessage({ id: "recently_used_galleries" })}</p>
          {recentGalleries.map((gallery) => (
            <Button
              key={gallery.id}
              variant="secondary"
              className="mr-2 mb-2"
              disabled={saving}
              onClick={() => addToGallery(gallery)}
            >
              {galleryTitle(gallery)}
            </Button>
          ))}
        </div>
      )}
      <GallerySelect
        isDisabled={saving}
        creatable={false}
        onSelect={(galleries) => {
          if (galleries[0]) void addToGallery(galleries[0]);
        }}
      />
    </ModalComponent>
  );
};
