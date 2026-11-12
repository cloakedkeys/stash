package generate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stretchr/testify/require"
)

// Use the existing test ffmpeg executable to capture the commands actually run,
// including every preview segment and both screenshot seek paths.
func TestRangeGenerationArguments(t *testing.T) {
	snapshotPath := filepath.Join("testdata", "whole_generate_args.json")
	snapshots := map[string][][]string{}
	if os.Getenv("UPDATE_RANGE_SNAPSHOTS") == "" {
		b, err := os.ReadFile(snapshotPath)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(b, &snapshots))
	}
	cases := []string{"preview", "short preview", "preview fallback", "marker video", "marker webp", "marker screenshot", "sprite", "cover", "explicit cover", "preview webp"}
	for _, part := range []bool{false, true} {
		for _, name := range cases {
			t.Run(strconv.FormatBool(part)+"/"+name, func(t *testing.T) {
				dir := t.TempDir()
				capture := filepath.Join(dir, "args.json")
				log := filepath.Join(dir, "commands.jsonl")
				t.Setenv("STASH_TEST_GENERATION_ARGS", capture)
				t.Setenv("STASH_TEST_GENERATION_LOG", log)
				exe, err := os.Executable()
				require.NoError(t, err)
				g := Generator{Encoder: ffmpeg.NewEncoder(exe), FFMpegConfig: threadsTestConfig{4}, LockManager: fsutil.NewReadLockManager(), ScenePaths: previewTestPaths{dir: dir}, FFProbe: previewTestProbe(func(string) (*ffmpeg.VideoFile, error) {
					return &ffmpeg.VideoFile{VideoStream: &ffmpeg.FFProbeStream{}}, nil
				})}
				start, end := 100.0, 110.0
				if part {
					g.Range = ffmpeg.StreamRange{Start: &start, End: &end}
				}
				ctx := g.LockManager.ReadLock(context.Background(), "input.mp4")
				defer ctx.Cancel()
				switch name {
				case "preview", "preview fallback":
					err = g.previewVideo("input.mp4", 640, 360, 200, PreviewOptions{Segments: 4, SegmentDuration: 2, ExcludeStart: "90%", Preset: "slow", Audio: true}, name == "preview fallback", false)(ctx, filepath.Join(dir, "out.mp4"))
				case "short preview":
					// A part shorter than the configured preview uses one bounded segment.
					if part {
						end = 103
					}
					err = g.previewVideo("input.mp4", 640, 360, 3, PreviewOptions{Segments: 4, SegmentDuration: 2, Preset: "slow"}, false, false)(ctx, filepath.Join(dir, "out.mp4"))
				case "marker video":
					err = g.markerPreviewVideo("input.mp4", 640, 360, ffmpeg.VideoCodecLibX264, false, sceneMarkerOptions{Seconds: 9.5, Duration: 20, Audio: true})(ctx, filepath.Join(dir, "out.mp4"))
				case "marker webp":
					err = g.sceneMarkerWebp("input.mp4", sceneMarkerOptions{Seconds: 9.5})(ctx, filepath.Join(dir, "out.webp"))
				case "marker screenshot":
					err = g.sceneMarkerScreenshot("input.mp4", SceneMarkerScreenshotOptions{Seconds: 9.5, Width: 640})(ctx, filepath.Join(dir, "out.jpg"))
				case "sprite":
					_, err = g.SpriteScreenshot(context.Background(), "input.mp4", 9.5, 160, false)
				case "cover", "explicit cover":
					var at *float64
					if name == "explicit cover" {
						value := 5.0
						if part {
							value += start
						}
						at = &value
					}
					_, err = g.Screenshot(context.Background(), "input.mp4", 640, 100, ScreenshotOptions{At: at})
				case "preview webp":
					err = g.previewVideoToImage("preview.mp4")(ctx, filepath.Join(dir, "out.webp"))
				}
				require.NoError(t, err)
				b, err := os.ReadFile(log)
				require.NoError(t, err)
				var commands [][]string
				for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
					var args []string
					require.NoError(t, json.Unmarshal([]byte(line), &args))
					if part && name != "preview webp" {
						seek, duration := 0.0, 0.0
						for i, arg := range args {
							if arg == "-ss" {
								v, e := strconv.ParseFloat(args[i+1], 64)
								require.NoError(t, e)
								seek += v
							}
							if arg == "-t" {
								duration, err = strconv.ParseFloat(args[i+1], 64)
								require.NoError(t, err)
							}
						}
						// The concat command has no source-file seek.
						if !strings.Contains(strings.Join(args, " "), "-f concat") {
							require.GreaterOrEqual(t, seek, start, "%v", args)
							require.Less(t, seek, end, "%v", args)
							require.Greater(t, duration, 0.0, "%v", args)
							require.LessOrEqual(t, seek+duration, end, "%v", args)
							if name == "cover" {
								require.Equal(t, 102.0, seek)
							}
							if name == "explicit cover" {
								require.Equal(t, 105.0, seek)
							}
							if strings.HasPrefix(name, "marker") || name == "sprite" {
								require.Equal(t, 109.5, seek)
							}
						}
					}
					// Random temporary filenames are irrelevant to ffmpeg behaviour.
					for i, arg := range args {
						if strings.HasPrefix(filepath.FromSlash(arg), dir) {
							if i == len(args)-1 {
								args[i] = "OUTPUT"
							} else {
								args[i] = "TEMP_INPUT"
							}
						}
					}
					commands = append(commands, args)
				}
				if !part {
					if os.Getenv("UPDATE_RANGE_SNAPSHOTS") != "" {
						snapshots[name] = commands
					} else {
						require.Equal(t, snapshots[name], commands)
					}
				}
			})
		}
	}
	if os.Getenv("UPDATE_RANGE_SNAPSHOTS") != "" {
		b, err := json.MarshalIndent(snapshots, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0755))
		require.NoError(t, os.WriteFile(snapshotPath, append(b, '\n'), 0600))
	}
}

func TestPartCoverRejectsFileTimeOutsideRange(t *testing.T) {
	start, end := 100.0, 110.0
	dir := t.TempDir()
	t.Setenv("STASH_TEST_GENERATION_ARGS", filepath.Join(dir, "args.json"))
	t.Setenv("STASH_TEST_GENERATION_LOG", filepath.Join(dir, "commands.jsonl"))
	exe, err := os.Executable()
	require.NoError(t, err)
	g := Generator{Range: ffmpeg.StreamRange{Start: &start, End: &end}, LockManager: fsutil.NewReadLockManager(), Encoder: ffmpeg.NewEncoder(exe), ScenePaths: previewTestPaths{dir: dir}}
	for _, at := range []float64{0, 99, 110, 111} {
		_, err := g.Screenshot(context.Background(), "input.mp4", 640, 200, ScreenshotOptions{At: &at})
		require.ErrorContains(t, err, "outside the scene range")
	}
}
