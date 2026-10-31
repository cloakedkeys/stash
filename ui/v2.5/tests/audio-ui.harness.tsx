import { Suspense } from "react";
import { createRoot } from "react-dom/client";
import { ApolloLink, ApolloProvider, Observable } from "@apollo/client";
import { MemoryRouter } from "react-router-dom";
import { IntlProvider } from "react-intl";
import * as GQL from "../src/core/generated-graphql";
import { getClient } from "../src/core/StashService";
import AudioRoutes from "../src/components/Audios/Audios";
import { ConfigurationProvider } from "../src/hooks/Config";
import { CurrentUserProvider } from "../src/hooks/CurrentUser";
import { ToastProvider } from "../src/hooks/Toast";
import flattenMessages from "../src/utils/flattenMessages";
import messages from "../src/locales/en-GB.json";
import "../src/index.scss";

const sampleRate = 8000;
const sampleCount = sampleRate * 2;
const wavBuffer = new ArrayBuffer(44 + sampleCount * 2);
const wavView = new DataView(wavBuffer);
const writeText = (offset: number, value: string) => {
  for (let i = 0; i < value.length; i++) {
    wavView.setUint8(offset + i, value.charCodeAt(i));
  }
};
writeText(0, "RIFF");
wavView.setUint32(4, wavBuffer.byteLength - 8, true);
writeText(8, "WAVE");
writeText(12, "fmt ");
wavView.setUint32(16, 16, true);
wavView.setUint16(20, 1, true);
wavView.setUint16(22, 1, true);
wavView.setUint32(24, sampleRate, true);
wavView.setUint32(28, sampleRate * 2, true);
wavView.setUint16(32, 2, true);
wavView.setUint16(34, 16, true);
writeText(36, "data");
wavView.setUint32(40, sampleCount * 2, true);
const wavBytes = new Uint8Array(wavBuffer);
let wavBinary = "";
for (let i = 0; i < wavBytes.length; i += 0x8000) {
  wavBinary += String.fromCharCode(...wavBytes.subarray(i, i + 0x8000));
}
const silentWav = `data:audio/wav;base64,${btoa(wavBinary)}`;

const poster =
  "data:image/svg+xml," +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"><defs><linearGradient id="g" x2="1" y2="1"><stop stop-color="#263c58"/><stop offset="1" stop-color="#151a29"/></linearGradient></defs><rect width="640" height="360" fill="url(#g)"/><circle cx="320" cy="180" r="104" fill="#d19b70" opacity=".17"/><g fill="#f5c896"><rect x="135" y="174" width="18" height="12" rx="9"/><rect x="170" y="150" width="18" height="60" rx="9"/><rect x="205" y="126" width="18" height="108" rx="9"/><rect x="240" y="158" width="18" height="44" rx="9"/><rect x="275" y="105" width="18" height="150" rx="9"/><rect x="310" y="135" width="18" height="90" rx="9"/><rect x="345" y="120" width="18" height="120" rx="9"/><rect x="380" y="158" width="18" height="44" rx="9"/><rect x="415" y="142" width="18" height="76" rx="9"/><rect x="450" y="170" width="18" height="20" rx="9"/></g><text x="320" y="318" fill="#f3eee9" font-family="sans-serif" font-size="20" text-anchor="middle" letter-spacing="4">FIELD RECORDINGS</text></svg>`
  );

const performers: GQL.AudioDataFragment["performers"] = [];

function makeAudio(
  id: string,
  title: string,
  studioName: string,
  date: string
) {
  const file = {
    __typename: "AudioFile",
    id: `file-${id}`,
    path: `fixture/field-recording-${id}.wav`,
    size: 32044,
    mod_time: "2026-10-30T12:00:00Z",
    created_at: "2026-10-30T12:00:00Z",
    updated_at: "2026-10-30T12:00:00Z",
    missing_since: null,
    format: "wav",
    duration: 2,
    audio_codec: "pcm_s16le",
    sample_rate: sampleRate,
    bit_rate: sampleRate * 16,
    fingerprints: [],
  };
  const studio = {
    __typename: "Studio",
    id: `studio-${id}`,
    name: studioName,
    image_path: null,
    details: null,
    rating100: null,
    aliases: [],
    favorite: false,
    ignore_auto_tag: false,
    organized: false,
    o_counter: 0,
    stash_ids: [],
    parent_studio: null,
    tags: [],
  };
  return {
    __typename: "Audio",
    id,
    title,
    code: `FIELD-${id}`,
    details:
      "A quiet, carefully recorded field piece with warm room tone and a measured pulse.",
    urls: [],
    date,
    rating100: 80,
    o_counter: 3,
    organized: false,
    created_at: "2026-10-01T12:00:00Z",
    updated_at: "2026-10-30T12:00:00Z",
    resume_time: 0,
    last_played_at: null,
    play_duration: 0,
    play_count: 0,
    play_history: [],
    o_history: [],
    custom_fields: {},
    files: [file],
    paths: {
      __typename: "AudioPathsType",
      screenshot: poster,
      stream: silentWav,
    },
    galleries: [],
    studio,
    groups: [],
    tags: [],
    performers,
    audioStreams: [
      {
        __typename: "AudioStreamEndpoint",
        url: silentWav,
        mime_type: "audio/wav",
        label: "Fixture WAV",
      },
    ],
  } as unknown as GQL.AudioDataFragment;
}

export const audios = [
  makeAudio("101", "Quiet Morning", "Blue Hour Studio", "2026-10-05"),
  makeAudio("102", "After the Rain", "North Window Audio", "2026-10-12"),
  makeAudio("103", "Midnight Signal", "Low Tide Records", "2026-10-19"),
];

const configuration = {
  __typename: "ConfigResult",
  general: { __typename: "ConfigGeneralResult", stashBoxes: [], stashes: [] },
  interface: {
    __typename: "ConfigInterfaceResult",
    language: "en-GB",
    continuePlaylistDefault: false,
    autostartVideoOnPlaySelected: false,
    maximumLoopDuration: 0,
    showStudioAsText: true,
  },
  ui: {
    minimumPlayPercent: 50,
    trackActivity: true,
  },
} as unknown as GQL.ConfigDataFragment;

// Replace the application's singleton link as well as its provider transport:
// StashService helpers intentionally share this client. No backend is contacted.
const requests: { name: string; variables: Record<string, unknown> }[] = [];
const client = getClient();
client.setLink(
  new ApolloLink(
    (operation) =>
      new Observable((observer) => {
        requests.push({
          name: operation.operationName,
          variables: operation.variables,
        });
        const vars = operation.variables;
        const audio = audios.find(
          (item) => item.id === (vars.id ?? vars.input?.id)
        );
        const data: Record<string, unknown> = {};
        const definition = operation.query.definitions.find(
          (item) => item.kind === "OperationDefinition"
        );
        if (definition?.kind !== "OperationDefinition") {
          observer.error(new Error("Fixture received no operation"));
          return;
        }
        for (const field of definition.selectionSet.selections) {
          if (field.kind !== "Field") continue;
          const name = field.name.value;
          const key = field.alias?.value ?? name;
          switch (name) {
            case "findAudios": {
              const items = audios.filter(
                (item) =>
                  (!vars.ids || vars.ids.includes(item.id)) &&
                  (!vars.filter?.q ||
                    item.title
                      ?.toLowerCase()
                      .includes(vars.filter.q.toLowerCase()))
              );
              data[key] = {
                __typename: "FindAudiosResultType",
                count: items.length,
                duration: items.length * 2,
                filesize: items.length * wavBytes.length,
                audios: items,
              };
              break;
            }
            case "findAudio":
              data[key] = audio ?? null;
              break;
            case "findSavedFilters":
              data[key] = [];
              break;
            case "findDefaultFilter":
              data[key] = null;
              break;
            case "configuration":
              data[key] = configuration;
              break;
            case "findPerformers":
              data[key] = { count: 0, performers: [] };
              break;
            case "findStudios":
              data[key] = { count: 0, studios: [] };
              break;
            case "findTags":
              data[key] = { count: 0, tags: [] };
              break;
            case "findGroups":
              data[key] = { count: 0, groups: [] };
              break;
            case "findGalleries":
              data[key] = { count: 0, galleries: [] };
              break;
            case "listScrapers":
              data[key] = [];
              break;
            case "audioSaveActivity":
              if (audio) {
                audio.resume_time = vars.resume_time ?? audio.resume_time;
                audio.play_duration += vars.playDuration ?? 0;
              }
              data[key] = true;
              break;
            case "audioAddPlay":
            case "audioAddO": {
              const history =
                name === "audioAddPlay"
                  ? audio?.play_history
                  : audio?.o_history;
              history?.push(new Date().toISOString());
              if (audio && history) {
                if (name === "audioAddPlay") audio.play_count = history.length;
                else audio.o_counter = history.length;
              }
              data[key] = {
                __typename: "HistoryMutationResult",
                count: history?.length ?? 0,
                history: history ?? [],
              };
              break;
            }
            case "audioUpdate":
              if (audio) Object.assign(audio, vars.input);
              data[key] = audio ?? null;
              break;
            case "configureUI":
              data[key] = { ...configuration.ui, ...vars.input };
              break;
            default:
              observer.error(new Error(`Unhandled fixture operation ${name}`));
              return;
          }
        }
        observer.next({ data: structuredClone(data) });
        observer.complete();
      })
  )
);
Object.assign(window, { audioFixture: { requests, audios } });

const path =
  new URLSearchParams(window.location.search).get("path") ?? "/audios";

createRoot(document.getElementById("fixture")!).render(
  <ApolloProvider client={client}>
    <MemoryRouter initialEntries={[path]}>
      <IntlProvider locale="en-GB" messages={flattenMessages(messages)}>
        <CurrentUserProvider
          user={{
            __typename: "User",
            name: "fixture-user",
            roles: [GQL.RoleEnum.Admin],
          }}
        >
          <ConfigurationProvider configuration={configuration}>
            <ToastProvider>
              <main className="main container-fluid">
                <Suspense fallback={null}>
                  <AudioRoutes />
                </Suspense>
              </main>
            </ToastProvider>
          </ConfigurationProvider>
        </CurrentUserProvider>
      </IntlProvider>
    </MemoryRouter>
  </ApolloProvider>
);
