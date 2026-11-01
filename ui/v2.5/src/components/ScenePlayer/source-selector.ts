import videojs, { VideoJsPlayer } from "video.js";

export interface ISource extends videojs.Tech.SourceObject {
  label?: string;
  errored?: boolean;
  streamType?: string;
  resolution?: string;
}

function preferredSourceIndex(
  sources: ISource[],
  preferredType: string | undefined,
  preferredResolution: string | undefined
): number {
  let matchedType = -1;
  for (let i = 0; i < sources.length; i++) {
    const source = sources[i];
    const matchType = preferredType
      ? source.streamType === preferredType
      : true;
    if (matchType && source.resolution === preferredResolution) return i;
    if (matchType && source.resolution === "ORIGINAL" && matchedType === -1)
      matchedType = i;
  }
  return matchedType === -1 ? 0 : matchedType;
}

class SourceMenuItem extends videojs.getComponent("MenuItem") {
  public source: ISource;
  public isSelected = false;

  constructor(parent: SourceMenuButton, source: ISource) {
    const options = {} as videojs.MenuItemOptions;
    options.selectable = true;
    options.multiSelectable = false;
    options.label = source.label || source.type;

    super(parent.player(), options);

    this.source = source;

    this.addClass("vjs-source-menu-item");
  }

  selected(selected: boolean): void {
    super.selected(selected);
    this.isSelected = selected;
  }

  handleClick() {
    if (this.isSelected) return;

    this.trigger("selected");
  }
}

class SourceMenuButton extends videojs.getComponent("MenuButton") {
  private items: SourceMenuItem[] = [];
  private selectedSource: ISource | null = null;

  constructor(player: VideoJsPlayer) {
    super(player);

    player.on("loadstart", () => {
      this.update();
    });
  }

  public setSources(sources: ISource[], selectedIndex: number) {
    this.selectedSource = null;

    this.items = sources.map((source, i) => {
      if (i === selectedIndex) {
        this.selectedSource = source;
      }

      const item = new SourceMenuItem(this, source);

      item.on("selected", () => {
        this.selectedSource = source;

        this.trigger("sourceselected", source);
      });

      return item;
    });
  }

  createEl() {
    return videojs.dom.createEl("div", {
      className:
        "vjs-source-selector vjs-menu-button vjs-menu-button-popup vjs-control vjs-button",
    });
  }

  createItems() {
    if (this.items === undefined) return [];

    for (const item of this.items) {
      item.selected(item.source === this.selectedSource);
    }

    return this.items;
  }

  setSelectedSource(source: ISource) {
    this.selectedSource = source;
    if (this.items === undefined) return;

    for (const item of this.items) {
      item.selected(item.source === this.selectedSource);
    }
  }

  markSourceErrored(source: ISource) {
    const item = this.items.find((i) => i.source.src === source.src);
    if (item === undefined) return;

    item.addClass("vjs-source-menu-item-error");
  }
}

class SourceSelectorPlugin extends videojs.getPlugin("plugin") {
  private menu: SourceMenuButton;
  private sources: ISource[] = [];
  private selectedIndex = -1;
  private cleanupTextTracks: HTMLTrackElement[] = [];
  private manualTextTracks: HTMLTrackElement[] = [];

  // don't auto play next source if user manually selected a source
  private manuallySelected = false;

  private sourceCheckURL: string | null | undefined;
  private availabilityCheck: Promise<boolean> | undefined;
  private sourceGeneration = 0;

  // returns whether playback is currently intended (autostart or already playing).
  // used to avoid auto-starting playback during source resolution/failover.
  private shouldAutoplay: () => boolean = () => false;

  constructor(player: VideoJsPlayer) {
    super(player);

    this.menu = new SourceMenuButton(player);

    this.menu.on("sourceselected", (_, source: ISource) => {
      this.selectedIndex = this.sources.indexOf(source);
      if (this.selectedIndex === -1) return;

      this.manuallySelected = true;

      const loadSrc = this.sources[this.selectedIndex];

      const currentTime = player.currentTime();
      const paused = player.paused();

      player.src(loadSrc);
      player.one("canplay", () => {
        if (paused) {
          player.pause();
        }
        player.currentTime(currentTime);
      });
      player.play();
    });

    player.on("ready", () => {
      const { controlBar } = player;
      const fullscreenToggle = controlBar.getChild("fullscreenToggle")!.el();
      controlBar.addChild(this.menu);
      controlBar.el().insertBefore(this.menu.el(), fullscreenToggle);
    });

    player.on("loadedmetadata", () => {
      if (!player.videoWidth() && !player.videoHeight()) {
        // Occurs during preload when videos with supported audio/unsupported video are preloaded.
        // Treat this as a decoding error and try the next source without playing.
        // However on Safari we get an media event when m3u8 or mpd is loaded which needs to be ignored.
        if (player.error() !== null) return;

        const currentSrc = player.currentSrc();
        if (currentSrc === null) return;

        if (currentSrc.includes(".m3u8") || currentSrc.includes(".mpd")) {
          if (this.shouldAutoplay()) {
            player.play();
          }
        } else {
          player.error(MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED);
          return;
        }
      }
    });

    player.on("error", async () => {
      const error = player.error();
      if (!error) return;

      // Only try next source if media was unsupported
      if (
        error.code !== MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED &&
        error.code !== MediaError.MEDIA_ERR_DECODE
      )
        return;

      // MediaError does not distinguish a missing file from an unsupported
      // codec. Check the direct stream before starting more encoders.
      const generation = this.sourceGeneration;
      const selectedIndex = this.selectedIndex;
      if (this.sourceCheckURL && !this.availabilityCheck) {
        this.availabilityCheck = fetch(this.sourceCheckURL, {
          method: "HEAD",
          cache: "no-store",
        })
          .then((response) => response.status !== 404)
          .catch(() => true);
      }
      if (this.availabilityCheck && !(await this.availabilityCheck)) return;
      // Multiple error events can share the pending check. Only the current
      // scene and source may advance once it finishes.
      if (
        player.isDisposed() ||
        generation !== this.sourceGeneration ||
        selectedIndex !== this.selectedIndex
      )
        return;

      const currentSource = player.currentSource() as ISource;
      console.log(`Source '${currentSource.label}' is unsupported`);

      // mark current source as errored in this.sources (currentSource() returns a copy)
      if (this.selectedIndex >= 0 && this.selectedIndex < this.sources.length) {
        this.sources[this.selectedIndex].errored = true;
      }
      this.menu.markSourceErrored(currentSource);

      // don't auto play next source if user manually selected a source
      if (this.manuallySelected) {
        return;
      }

      // TODO - make auto play next source configurable
      // try the next source in the list
      const nextIndex = (this.selectedIndex + 1) % this.sources.length;
      if (this.selectedIndex !== -1 && !this.sources[nextIndex].errored) {
        this.selectedIndex = nextIndex;
        const newSource = this.sources[nextIndex];
        console.log(`Trying next source in playlist: '${newSource.label}'`);
        this.menu.setSelectedSource(newSource);

        const currentTime = player.currentTime();
        player.src(newSource);
        player.load();
        player.one("canplay", () => {
          player.currentTime(currentTime);
        });
        if (this.shouldAutoplay()) {
          player.play();
        }
      } else {
        console.log("No more sources in playlist");
      }
    });
  }

  setSources(
    sources: ISource[],
    preferredType: string | undefined,
    preferredResolution: string | undefined,
    sourceCheckURL?: string | null
  ) {
    this.sourceGeneration++;
    this.sourceCheckURL = sourceCheckURL;
    this.availabilityCheck = undefined;

    const cleanupTracks = this.cleanupTextTracks.splice(0);
    for (const track of cleanupTracks) {
      this.player.removeRemoteTextTrack(track);
    }

    const selectedIndex = preferredSourceIndex(
      sources,
      preferredType,
      preferredResolution
    );

    this.menu.setSources(sources, selectedIndex);
    if (sources.length !== 0) {
      this.selectedIndex = selectedIndex;
    } else {
      this.selectedIndex = -1;
    }

    this.sources = sources;
    this.player.src(sources[selectedIndex]);
  }

  setShouldAutoplay(fn: () => boolean) {
    this.shouldAutoplay = fn;
  }

  get textTracks(): HTMLTrackElement[] {
    return [...this.cleanupTextTracks, ...this.manualTextTracks];
  }

  addTextTrack(options: videojs.TextTrackOptions, manualCleanup: boolean) {
    const track = this.player.addRemoteTextTrack(options, true);
    if (manualCleanup) {
      this.manualTextTracks.push(track);
    } else {
      this.cleanupTextTracks.push(track);
    }

    return track;
  }

  selectTextTrack() {
    // Video.js records mode changes as language choices synchronously. Automatic
    // fallback must not replace a saved choice for the next scene.
    const cache = this.player.getCache();
    const preference = cache.selectedLanguage;
    const display = this.player.getChild("textTrackDisplay") as
      | (videojs.Component & { preselectTrack(): void })
      | undefined;
    display?.preselectTrack();
    if (preference) cache.selectedLanguage = preference;
  }

  removeTextTrack(track: HTMLTrackElement) {
    this.player.removeRemoteTextTrack(track);
    let index = this.manualTextTracks.indexOf(track);
    if (index !== -1) {
      this.manualTextTracks.splice(index, 1);
    }
    index = this.cleanupTextTracks.indexOf(track);
    if (index !== -1) {
      this.cleanupTextTracks.splice(index, 1);
    }
  }
}

// Register the plugin with video.js.
videojs.registerComponent("SourceMenuButton", SourceMenuButton);
videojs.registerPlugin("sourceSelector", SourceSelectorPlugin);

declare module "video.js" {
  interface VideoJsPlayer {
    sourceSelector: () => SourceSelectorPlugin;
  }
  interface VideoJsPlayerPluginOptions {
    sourceSelector?: object;
  }
}

export default SourceSelectorPlugin;
