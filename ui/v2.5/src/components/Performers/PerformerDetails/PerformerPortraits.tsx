import React, { useRef, useState } from "react";
import { Alert, Button } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import {
  usePerformerImageAdd,
  usePerformerImageRemove,
  usePerformerImageSetPrimary,
  usePerformerImagesReorder,
} from "src/core/StashService";
import { ImageInput } from "src/components/Shared/ImageInput";
import { ModalComponent } from "src/components/Shared/Modal";
import { useCanModify } from "src/hooks/CurrentUser";
import { LightboxLink } from "src/hooks/Lightbox/LightboxLink";
import { useToast } from "src/hooks/Toast";
import { addPerformerImageFiles, movePerformerImage } from "./performerImages";

const pageSize = 12;

export const PerformerPortraits: React.FC<{
  performer: NonNullable<GQL.FindPerformerQuery["findPerformer"]>;
}> = ({ performer }) => {
  const intl = useIntl();
  const Toast = useToast();
  const canModify = useCanModify();
  const [addImage] = usePerformerImageAdd();
  const [removeImage] = usePerformerImageRemove();
  const [setPrimary] = usePerformerImageSetPrimary();
  const [reorder] = usePerformerImagesReorder();
  const [busy, setBusy] = useState(false);
  const [fileErrors, setFileErrors] = useState<string[]>([]);
  const running = useRef(false);
  const [removing, setRemoving] = useState<string>();
  const [page, setPage] = useState(0);
  const alternates = performer.alternate_images;
  const lastPage = Math.max(0, Math.ceil(alternates.length / pageSize) - 1);
  const currentPage = Math.min(page, lastPage);
  const start = currentPage * pageSize;
  const lightboxImages = [
    { paths: { image: performer.image_path, thumbnail: performer.image_path } },
    ...alternates.map((image) => ({
      paths: { image: image.path, thumbnail: image.path },
    })),
  ];

  async function run(action: () => Promise<unknown>) {
    if (!canModify || running.current) return;
    running.current = true;
    setBusy(true);
    try {
      await action();
    } catch (error) {
      Toast.error(error);
    } finally {
      running.current = false;
      setBusy(false);
    }
  }

  function add(image: string) {
    return addImage({ variables: { id: performer.id, image } });
  }

  function onFiles(event: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    if (running.current) return;
    setFileErrors([]);
    void run(() =>
      addPerformerImageFiles(files, add, (file, error) => {
        const message = intl.formatMessage(
          { id: "performer_portraits.file_error" },
          {
            name: file.name,
            error: error instanceof Error ? error.message : String(error),
          }
        );
        setFileErrors((errors) => [...errors, message]);
      })
    );
  }

  if (!alternates.length && !canModify) return null;

  return (
    <section
      className="performer-portraits mb-3"
      aria-label={intl.formatMessage({ id: "performer_portraits.title" })}
      aria-busy={busy}
    >
      {canModify && (
        <fieldset disabled={busy}>
          <ImageInput
            isEditing
            multiple
            text={intl.formatMessage({ id: "performer_portraits.add" })}
            onImageChange={onFiles}
            onImageURL={(image) => void run(() => add(image))}
          />
        </fieldset>
      )}
      {busy && <FormattedMessage id="performer_portraits.saving" />}
      {!!fileErrors.length && (
        <Alert variant="danger" aria-live="polite">
          <ul className="mb-0">
            {fileErrors.map((error, index) => (
              <li key={index}>{error}</li>
            ))}
          </ul>
        </Alert>
      )}
      {!!alternates.length && (
        <div className="d-flex flex-wrap">
          {alternates.slice(start, start + pageSize).map((image, offset) => {
            const index = start + offset;
            return (
              <div key={image.checksum} className="performer-portrait m-1">
                <LightboxLink images={lightboxImages} index={index + 1}>
                  <img
                    src={image.path}
                    alt={intl.formatMessage(
                      { id: "performer_portraits.image" },
                      { name: performer.name, number: index + 1 }
                    )}
                    loading="lazy"
                    decoding="async"
                    width={120}
                    height={160}
                    style={{ objectFit: "contain", maxWidth: "100%" }}
                  />
                </LightboxLink>
                {canModify && (
                  <div className="d-flex flex-column">
                    <Button
                      size="sm"
                      disabled={busy}
                      onClick={() =>
                        void run(() =>
                          setPrimary({
                            variables: {
                              id: performer.id,
                              checksum: image.checksum,
                            },
                          })
                        )
                      }
                    >
                      <FormattedMessage id="performer_portraits.make_main" />
                    </Button>
                    <div className="d-flex">
                      {([-1, 1] as const).map((direction) => (
                        <Button
                          key={direction}
                          size="sm"
                          className="flex-fill"
                          disabled={
                            busy ||
                            (direction === -1
                              ? index === 0
                              : index === alternates.length - 1)
                          }
                          aria-label={intl.formatMessage({
                            id:
                              direction === -1
                                ? "performer_portraits.move_left"
                                : "performer_portraits.move_right",
                          })}
                          onClick={() =>
                            void run(() =>
                              reorder({
                                variables: {
                                  id: performer.id,
                                  checksums: movePerformerImage(
                                    alternates.map((item) => item.checksum),
                                    index,
                                    direction
                                  ),
                                },
                              })
                            )
                          }
                        >
                          {direction === -1 ? "←" : "→"}
                        </Button>
                      ))}
                    </div>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={busy}
                      onClick={() => setRemoving(image.checksum)}
                    >
                      <FormattedMessage id="actions.remove" />
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
      {lastPage > 0 && (
        <div className="d-flex align-items-center">
          <Button
            disabled={busy || currentPage === 0}
            onClick={() => setPage(currentPage - 1)}
          >
            <FormattedMessage id="pagination.previous" />
          </Button>
          <span className="mx-2" aria-live="polite">
            <FormattedMessage
              id="pagination.current_total"
              values={{ current: currentPage + 1, total: lastPage + 1 }}
            />
          </span>
          <Button
            disabled={busy || currentPage === lastPage}
            onClick={() => setPage(currentPage + 1)}
          >
            <FormattedMessage id="pagination.next" />
          </Button>
        </div>
      )}
      {canModify && removing && (
        <ModalComponent
          show
          header={intl.formatMessage({ id: "actions.remove" })}
          isRunning={busy}
          onHide={() => {
            if (!busy) setRemoving(undefined);
          }}
          cancel={{ onClick: () => setRemoving(undefined) }}
          accept={{
            text: intl.formatMessage({ id: "actions.remove" }),
            variant: "danger",
            onClick: () =>
              void run(async () => {
                await removeImage({
                  variables: { id: performer.id, checksum: removing },
                });
                setRemoving(undefined);
              }),
          }}
        >
          <FormattedMessage id="performer_portraits.confirm_remove" />
        </ModalComponent>
      )}
    </section>
  );
};
