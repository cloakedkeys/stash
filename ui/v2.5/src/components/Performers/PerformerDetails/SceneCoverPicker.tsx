import React, { useRef, useState } from "react";
import { Button } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { objectTitle } from "src/core/files";
import { ModalComponent } from "src/components/Shared/Modal";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { ErrorMessage } from "src/components/Shared/ErrorMessage";
import { useToast } from "src/hooks/Toast";
import ImageUtils from "src/utils/image";

const pageSize = 12;

// A scene's cover as image data, read in the browser so the server isn't
// asked to fetch its own address. A cover that fails to load is an error,
// not a broken picture.
export async function coverToDataURL(url: string): Promise<string> {
  const response = await fetch(url);
  if (!response.ok) throw new Error(response.statusText);
  return ImageUtils.blobToDataURL(await response.blob());
}

export const SceneCoverPicker: React.FC<{
  performerId: string;
  onSelect: (image: string) => void;
  onClose: () => void;
}> = ({ performerId, onSelect, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [page, setPage] = useState(1);
  const [loadingImage, setLoadingImage] = useState(false);
  const busy = useRef(false);
  const { data, loading, error } = GQL.useFindScenesQuery({
    variables: {
      filter: {
        page,
        per_page: pageSize,
        sort: "date",
        direction: GQL.SortDirectionEnum.Desc,
      },
      scene_filter: {
        performers: {
          value: [performerId],
          modifier: GQL.CriterionModifier.Includes,
        },
        NOT: { is_missing: "cover" },
      },
    },
  });

  async function selectCover(url: string | null | undefined) {
    if (!url || busy.current) return;
    busy.current = true;
    setLoadingImage(true);
    try {
      const image = await coverToDataURL(url);
      onSelect(image);
      onClose();
    } catch {
      Toast.error(
        intl.formatMessage({ id: "performer_picture.cover_unavailable" })
      );
    } finally {
      busy.current = false;
      setLoadingImage(false);
    }
  }

  const scenes = data?.findScenes.scenes ?? [];
  const close = () => {
    if (!busy.current) onClose();
  };

  return (
    <ModalComponent
      show
      // a search or picker: a typed query is nothing to lose
      escapeAfterChanges
      header={intl.formatMessage({ id: "actions.set_from_scene_cover" })}
      onHide={close}
      accept={{ onClick: close }}
      isRunning={loadingImage}
    >
      <p>
        <FormattedMessage id="performer_picture.cover_save_hint" />
      </p>
      {loading ? (
        <LoadingIndicator />
      ) : error ? (
        <ErrorMessage error={error.message} />
      ) : (
        <>
          {!scenes.length && (
            <p>
              <FormattedMessage id="performer_picture.no_scene_covers" />
            </p>
          )}
          <div className="row">
            {scenes.map((scene) => (
              <div className="col-6 col-md-4 mb-3" key={scene.id}>
                <Button
                  type="button"
                  variant="secondary"
                  className="w-100"
                  disabled={loadingImage}
                  onClick={() => selectCover(scene.paths.screenshot)}
                >
                  <img
                    className="w-100"
                    src={ImageUtils.getThumbnailUrl(scene.paths.screenshot)}
                    alt={objectTitle(scene)}
                  />
                  <span>{objectTitle(scene)}</span>
                </Button>
              </div>
            ))}
          </div>
        </>
      )}
      <div className="d-flex justify-content-between">
        <Button
          type="button"
          disabled={page === 1 || loading || loadingImage}
          onClick={() => setPage(page - 1)}
        >
          <FormattedMessage id="pagination.previous" />
        </Button>
        <Button
          type="button"
          disabled={
            loading ||
            loadingImage ||
            page * pageSize >= (data?.findScenes.count ?? 0)
          }
          onClick={() => setPage(page + 1)}
        >
          <FormattedMessage id="pagination.next" />
        </Button>
      </div>
    </ModalComponent>
  );
};
