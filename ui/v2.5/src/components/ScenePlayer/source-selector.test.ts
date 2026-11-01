import { describe, expect, it } from "vitest";
import videojs, { type VideoJsPlayer } from "video.js";
import SourceSelectorPlugin from "./source-selector";

// Exercise the real Video.js selection rules without constructing a DOM player.
const Display = videojs.getComponent("TextTrackDisplay");
const MenuItem = videojs.getComponent("TextTrackMenuItem");
const recordLanguage = (
  MenuItem.prototype as videojs.Component & {
    handleSelectedLanguageChange(): void;
  }
).handleSelectedLanguageChange;
const preselectTrack = (
  Display.prototype as videojs.Component & { preselectTrack(): void }
).preselectTrack;

function setup() {
  const tracks: TextTrack[] = [];
  const cache: {
    selectedLanguage?: { enabled: boolean; language?: string; kind?: string };
  } = {};
  const player = {
    cache_: cache,
    getCache: () => cache,
    textTracks: () => tracks,
    getChild: () => ({
      preselectTrack: () => preselectTrack.call({ player_: player }),
    }),
    addRemoteTextTrack: (options: videojs.TextTrackOptions) => {
      const track = {
        kind: options.kind,
        language: options.srclang,
        default: options.default,
      } as unknown as TextTrack;
      let mode = "disabled";
      Object.defineProperty(track, "mode", {
        get: () => mode,
        set: (value: string) => {
          if (mode === value) return;
          mode = value;
          // Video.js emits selectedlanguagechange synchronously on modechange.
          for (const item of tracks) {
            recordLanguage.call({ player_: player, track: item });
          }
        },
      });
      tracks.push(track);
      return { track } as HTMLTrackElement;
    },
    removeRemoteTextTrack: (element: HTMLTrackElement) => {
      tracks.splice(tracks.indexOf(element.track), 1);
    },
    src: () => {},
  };
  const selector = Object.assign(
    Object.create(SourceSelectorPlugin.prototype),
    {
      player: player as unknown as VideoJsPlayer,
      cleanupTextTracks: [],
      manualTextTracks: [],
      sourceGeneration: 0,
      menu: { setSources: () => {} },
    }
  ) as SourceSelectorPlugin;
  const add = (language: string, isDefault = false) =>
    selector.addTextTrack(
      { kind: "captions", srclang: language, default: isDefault },
      false
    ).track;
  const nextScene = () => selector.setSources([], undefined, undefined);
  const selectAtReadiness = () => preselectTrack.call({ player_: player });
  const finish = () => selector.selectTextTrack();
  return { add, nextScene, tracks, cache, selector, selectAtReadiness, finish };
}

describe("caption selection when tracks are added after player readiness", () => {
  it.each([
    "srt",
    "vtt",
  ])("selects a default external %s track again on A → B → A before metadata loads", () => {
    const { add, nextScene, tracks, selector, selectAtReadiness, finish } =
      setup();
    const first = add("en", true);
    selectAtReadiness();
    expect(first.mode).toBe("showing");
    nextScene();
    expect(tracks).toEqual([]);
    expect(selector.textTracks).toEqual([]);
    expect(add("fr").mode).toBe("disabled");
    nextScene();
    const back = add("en", true);
    finish();
    expect(back).not.toBe(first);
    expect(back.mode).toBe("showing");
    expect(tracks).toEqual([back]);
  });

  it("keeps a saved Off choice for default and late embedded tracks", () => {
    const { add, nextScene, cache, finish } = setup();
    add("en", true);
    cache.selectedLanguage = { enabled: false };
    nextScene();
    expect(add("en", true).mode).toBe("disabled");
    expect(add("en").mode).toBe("disabled");
    finish();
    expect(cache.selectedLanguage).toEqual({ enabled: false });
  });

  it("keeps French until the external and embedded tracks have all arrived on Back", () => {
    const { add, nextScene, cache, finish } = setup();
    const english = add("en", true);
    const french = add("fra");
    finish();
    // A manual choice follows the same synchronous Video.js cache writer.
    english.mode = "disabled";
    french.mode = "showing";
    const choice = { enabled: true, language: "fra", kind: "captions" };
    expect(cache.selectedLanguage).toEqual(choice);
    nextScene();
    nextScene();
    const externalEnglish = add("en", true);
    expect(cache.selectedLanguage).toEqual(choice);
    expect(externalEnglish.mode).toBe("disabled");
    const embeddedEnglish = add("eng");
    const embeddedFrench = add("fra");
    finish();
    expect(externalEnglish.mode).toBe("disabled");
    expect(embeddedEnglish.mode).toBe("disabled");
    expect(embeddedFrench.mode).toBe("showing");
    expect(cache.selectedLanguage).toEqual(choice);
  });

  it("uses the default after registration without losing an unavailable language", () => {
    const { add, cache, finish } = setup();
    const choice = { enabled: true, language: "fra", kind: "captions" };
    cache.selectedLanguage = choice;
    const english = add("en", true);
    add("eng");
    finish();
    expect(english.mode).toBe("showing");
    expect(cache.selectedLanguage).toEqual(choice);
  });

  it("leaves unmatched external and opt-in embedded captions disabled", () => {
    const { add, finish } = setup();
    const french = add("fr");
    const english = add("en");
    finish();
    expect(french.mode).toBe("disabled");
    expect(english.mode).toBe("disabled");
  });
});
