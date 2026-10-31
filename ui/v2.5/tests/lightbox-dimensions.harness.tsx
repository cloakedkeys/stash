// Browser regression fixture: import from a running Vite app, then call renderCase.
// A smaller decoded JPEG simulates Chrome's low-memory image downsampling while
// retaining the original dimensions used by the lightbox geometry.
import { createRoot } from "react-dom/client";
import { IntlProvider } from "react-intl";
import * as GQL from "../src/core/generated-graphql";
import { LightboxImage } from "../src/hooks/Lightbox/LightboxImage";

let root: ReturnType<typeof createRoot> | undefined;

export async function renderCase(
  width: number,
  height: number,
  decodedWidth: number,
  decodedHeight: number,
  displayMode = GQL.ImageLightboxDisplayMode.FitXy
) {
  root?.unmount();
  document.getElementById("lightbox-regression")?.remove();
  const host = document.createElement("div");
  host.id = "lightbox-regression";
  Object.assign(host.style, {
    position: "fixed",
    inset: "0",
    zIndex: "10000",
    background: "black",
  });
  document.body.append(host);
  const canvas = document.createElement("canvas");
  canvas.width = decodedWidth;
  canvas.height = decodedHeight;
  const ctx = canvas.getContext("2d")!;
  ctx.fillStyle = "#157f99";
  ctx.fillRect(0, 0, decodedWidth, decodedHeight);
  ctx.fillStyle = "#ffd479";
  ctx.fillRect(
    decodedWidth / 4,
    decodedHeight / 4,
    decodedWidth / 2,
    decodedHeight / 2
  );
  const src = canvas.toDataURL("image/jpeg");
  root = createRoot(host);
  root.render(
    <IntlProvider locale="en" messages={{}}>
      <div
        className="Lightbox-carousel"
        style={{ width: "100vw", height: "100vh", left: 0 }}
      >
        <LightboxImage
          key={`${width}-${height}-${displayMode}`}
          src={src}
          width={width}
          height={height}
          displayMode={displayMode}
          scaleUp={false}
          scrollMode={GQL.ImageLightboxScrollMode.Zoom}
          zoom={1}
          scrollAttemptsBeforeChange={2}
          firstScroll={{ current: null }}
          inScrollGroup={{ current: false }}
          current={true}
          setZoom={() => {}}
          debouncedScrollReset={() => {}}
          onLeft={() => {}}
          onRight={() => {}}
          navigationEnabled={false}
          isVideo={false}
        />
      </div>
    </IntlProvider>
  );
  await new Promise((resolve) => setTimeout(resolve, 500));
  const image = host.querySelector("img")!;
  await image.decode();
  const r = image.getBoundingClientRect();
  return {
    natural: [image.naturalWidth, image.naturalHeight],
    bounds: { x: r.x, y: r.y, width: r.width, height: r.height },
    centered:
      Math.abs(r.x + r.width / 2 - innerWidth / 2) < 2 &&
      Math.abs(r.y + r.height / 2 - innerHeight / 2) < 2,
    visible:
      r.right > 0 && r.left < innerWidth && r.bottom > 0 && r.top < innerHeight,
  };
}
