import React, {
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useApolloClient } from "@apollo/client";
import { Link } from "react-router-dom";
import { FormattedMessage, useIntl } from "react-intl";
import { Button } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import { isVideo } from "src/utils/visualFile";
import {
  ReaderBatches,
  ReaderDimensions,
  readerLayout,
} from "src/models/galleryReader";

type ReaderImage = GQL.GalleryReaderImagesQuery["findImages"]["images"][number];

const PageImage: React.FC<{
  image: ReaderImage;
  number: number;
  onDimensions: (dimensions: ReaderDimensions) => void;
}> = ({ image, number, onDimensions }) => {
  const intl = useIntl();
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [attempt, setAttempt] = useState(0);

  if (isVideo(image.visual_files[0] ?? {})) {
    return (
      <video
        src={image.paths.image ?? undefined}
        poster={image.paths.thumbnail ?? undefined}
        muted
        controls
        preload="none"
        aria-label={intl.formatMessage(
          { id: "gallery_reader.page" },
          { number }
        )}
        style={{
          display: "block",
          width: "100%",
          height: "100%",
          maxHeight: "calc(100dvh - 4rem)",
          objectFit: "contain",
        }}
      />
    );
  }

  return (
    <>
      {!failed && image.paths.image && (
        <img
          key={attempt}
          src={image.paths.image}
          alt={intl.formatMessage({ id: "gallery_reader.page" }, { number })}
          style={{
            display: "block",
            width: "100%",
            height: "100%",
            objectFit: "contain",
          }}
          onLoad={(event) => {
            setLoaded(true);
            onDimensions({
              width: event.currentTarget.naturalWidth,
              height: event.currentTarget.naturalHeight,
            });
          }}
          onError={() => setFailed(true)}
        />
      )}
      {(!loaded || failed) && (
        <div
          style={{ position: "absolute", top: 0, left: 0, padding: "1rem" }}
          role="status"
        >
          {failed || !image.paths.image ? (
            <>
              <FormattedMessage
                id="gallery_reader.image_error"
                values={{ number }}
              />{" "}
              <Button
                onClick={() => {
                  setLoaded(false);
                  setFailed(false);
                  setAttempt((value) => value + 1);
                }}
              >
                <FormattedMessage id="gallery_reader.retry" />
              </Button>
            </>
          ) : (
            <FormattedMessage id="gallery_reader.loading" />
          )}
        </div>
      )}
    </>
  );
};

// Keyed by gallery in the loader, so navigating to another gallery starts a
// fresh window. Late responses only update their original batch controller.
export const GalleryReader: React.FC<{ galleryId: string }> = ({
  galleryId,
}) => {
  const client = useApolloClient();
  const intl = useIntl();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState({ width: 0, height: 0, top: 0 });
  const [dimensions, setDimensions] = useState<
    Record<string, ReaderDimensions>
  >({});
  const reader = useMemo(
    () =>
      new ReaderBatches<ReaderImage>(async (page, perPage) => {
        const result = await client.query<
          GQL.GalleryReaderImagesQuery,
          GQL.GalleryReaderImagesQueryVariables
        >({
          query: GQL.GalleryReaderImagesDocument,
          fetchPolicy: "no-cache",
          variables: {
            filter: {
              page,
              per_page: perPage,
              sort: "path",
              direction: GQL.SortDirectionEnum.Asc,
            },
            image_filter: {
              galleries: {
                modifier: GQL.CriterionModifier.Includes,
                value: [galleryId],
              },
            },
          },
        });
        return result.data.findImages;
      }),
    [client, galleryId]
  );
  const state = useSyncExternalStore(reader.subscribe, reader.getSnapshot);
  const sizes = state.images.map((image) => {
    const file = image.visual_files[0];
    return (
      dimensions[image.id] ?? {
        width: file && "width" in file ? file.width : 0,
        height: file && "height" in file ? file.height : 0,
      }
    );
  });
  const layout = readerLayout(
    sizes,
    viewport.width,
    viewport.top,
    viewport.height
  );
  const totalHeight = layout.reduce((sum, page) => sum + page.height, 0);

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    element.focus();
    const measure = () =>
      setViewport({
        width: element.clientWidth,
        height: element.clientHeight,
        top: element.scrollTop,
      });
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    measure();
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (
      viewport.height > 0 &&
      !state.loading &&
      !state.error &&
      !state.ended &&
      viewport.top + viewport.height * 3 >= totalHeight
    ) {
      void reader.loadNext();
    }
  }, [
    reader,
    state.loading,
    state.error,
    state.ended,
    totalHeight,
    viewport.height,
    viewport.top,
  ]);

  return (
    <section
      className="bg-dark text-light"
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 1050,
        display: "flex",
        flexDirection: "column",
      }}
      aria-label={intl.formatMessage({ id: "gallery_reader.read" })}
    >
      <div style={{ padding: "0.5rem" }}>
        <Link to={`/galleries/${galleryId}`} className="btn btn-secondary">
          <FormattedMessage id="gallery_reader.back" />
        </Link>
      </div>
      <div
        ref={scrollRef}
        tabIndex={0}
        role="region"
        aria-label={intl.formatMessage({ id: "gallery_reader.read" })}
        style={{
          overflowY: "auto",
          flex: 1,
          minHeight: 0,
          overscrollBehavior: "contain",
        }}
        onScroll={(event) => {
          const top = event.currentTarget.scrollTop;
          setViewport((previous) => ({ ...previous, top }));
        }}
      >
        {layout.map((page) => {
          const image = state.images[page.index];
          return (
            <div
              key={image.id}
              style={{ height: page.height, position: "relative" }}
            >
              {page.mounted && (
                <PageImage
                  image={image}
                  number={page.index + 1}
                  onDimensions={(size) => {
                    // Use the file dimensions whenever available; remember
                    // natural dimensions for older files with missing sizes.
                    if (
                      sizes[page.index].width <= 0 ||
                      sizes[page.index].height <= 0
                    ) {
                      setDimensions((previous) => ({
                        ...previous,
                        [image.id]: size,
                      }));
                    }
                  }}
                />
              )}
            </div>
          );
        })}
        <div role="status" style={{ padding: "1rem", textAlign: "center" }}>
          {state.loading && <FormattedMessage id="gallery_reader.loading" />}
          {state.error && (
            <>
              <FormattedMessage id="gallery_reader.batch_error" />{" "}
              <Button onClick={() => void reader.loadNext()}>
                <FormattedMessage id="gallery_reader.retry" />
              </Button>
            </>
          )}
          {state.ended && <FormattedMessage id="gallery_reader.end" />}
        </div>
      </div>
    </section>
  );
};
