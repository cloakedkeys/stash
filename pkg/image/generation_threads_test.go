package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stretchr/testify/require"
)

func init() {
	if path := os.Getenv("STASH_TEST_CLIP_ARGS"); path != "" {
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

func TestClipPreviewGenerationThreads(t *testing.T) {
	for _, threads := range []int{7, 0} {
		t.Run(fmt.Sprint(threads), func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "args.json")
			t.Setenv("STASH_TEST_CLIP_ARGS", capture)
			exe, err := os.Executable()
			require.NoError(t, err)
			encoder := NewThumbnailEncoder(ffmpeg.NewEncoder(exe), nil, ClipPreviewOptions{Threads: threads, Preset: "slow", OutputArgs: []string{"-threads", "2"}})
			require.NoError(t, encoder.getClipPreview("input.mp4", filepath.Join(t.TempDir(), "out.webm"), 320, 1, 30, nil))
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
			require.Equal(t, []string{fmt.Sprint(threads), "2"}, values, "args: %v", args)
		})
	}
}
