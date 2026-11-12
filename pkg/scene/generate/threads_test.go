package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stretchr/testify/require"
)

// The test executable stands in for ffmpeg so these tests inspect the complete command.
func init() {
	if path := os.Getenv("STASH_TEST_GENERATION_ARGS"); path != "" {
		b, err := json.Marshal(os.Args[1:])
		if err != nil {
			os.Exit(1)
		}
		if os.WriteFile(path, b, 0600) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

type threadsTestConfig struct{ threads int }

func (c threadsTestConfig) GetGenerationThreads() int             { return c.threads }
func (threadsTestConfig) GetTranscodeInputArgs() []string         { return nil }
func (threadsTestConfig) GetTranscodeOutputArgs() []string        { return []string{"-threads", "2"} }
func (threadsTestConfig) GetGenerationHardwareAcceleration() bool { return false }

func TestNegativeGenerationThreads(t *testing.T) {
	c := config.InitializeEmpty()
	c.SetInterface(config.GenerationThreads, -3)
	g := Generator{Encoder: ffmpeg.NewEncoder("ffmpeg"), FFMpegConfig: c}
	args, _ := g.transcodeVideoArgs(ffmpeg.VideoCodecLibX264, false, 640, 360, TranscodeOptions{})
	for i, arg := range args {
		if arg == "-threads" {
			require.Less(t, i+1, len(args))
			require.Equal(t, "0", args[i+1], "args: %v", args)
			return
		}
	}
	t.Fatalf("missing -threads in args: %v", args)
}

func TestGenerationThreads(t *testing.T) {
	for _, threads := range []int{7, 0} {
		for _, codec := range []ffmpeg.VideoCodec{ffmpeg.VideoCodecLibX264, ffmpeg.VideoCodecVP9} {
			for _, site := range []string{"preview video", "preview webp", "marker video", "marker webp", "transcode", "video transcode"} {
				if codec == ffmpeg.VideoCodecVP9 && site != "preview video" && site != "marker video" {
					continue
				}
				t.Run(site+"/"+codec.Name+"/"+fmt.Sprint(threads), func(t *testing.T) {
					capture := filepath.Join(t.TempDir(), "args.json")
					t.Setenv("STASH_TEST_GENERATION_ARGS", capture)
					exe, err := os.Executable()
					require.NoError(t, err)
					g := Generator{Encoder: ffmpeg.NewEncoder(exe), FFMpegConfig: threadsTestConfig{threads}}
					ctx := fsutil.NewReadLockManager().ReadLock(context.Background(), "input.mp4")
					defer ctx.Cancel()
					switch site {
					case "preview video":
						err = g.previewVideoChunk(ctx, "input.mp4", 640, 360, previewChunkOptions{Preset: "slow", OutputPath: "out.mp4"}, false, false, codec, false)
					case "preview webp":
						err = g.previewVideoToImage("input.mp4")(ctx, "out.webp")
					case "marker video":
						err = g.markerPreviewVideo("input.mp4", 640, 360, codec, false, sceneMarkerOptions{})(ctx, "out.mp4")
					case "marker webp":
						err = g.sceneMarkerWebp("input.mp4", sceneMarkerOptions{})(ctx, "out.webp")
					case "transcode":
						err = g.transcode("input.mp4", 640, 360, TranscodeOptions{})(ctx, "out.mp4")
					case "video transcode":
						err = g.transcodeVideo("input.mp4", 640, 360, TranscodeOptions{})(ctx, "out.mp4")
					}
					require.NoError(t, err)
					b, err := os.ReadFile(capture)
					require.NoError(t, err)
					var args []string
					require.NoError(t, json.Unmarshal(b, &args))
					var values []string
					for i, arg := range args {
						if arg == "-threads" {
							require.Less(t, i+1, len(args))
							values = append(values, args[i+1])
						}
					}
					want := []string{fmt.Sprint(threads)}
					if site != "marker video" && site != "marker webp" {
						want = append(want, "2")
					}
					require.Equal(t, want, values, "args: %v", args)
				})
			}
		}
	}
}
