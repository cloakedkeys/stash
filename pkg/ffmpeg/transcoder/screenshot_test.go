package transcoder

import (
	"reflect"
	"testing"
)

func TestScreenshotTimeDefaultUsesFastSeek(t *testing.T) {
	options := ScreenshotOptions{
		OutputPath: "out.jpg",
		OutputType: ScreenshotOutputTypeImage2,
	}

	got := ScreenshotTime("input.webm", 12.5, options)
	want := []string{
		"-v", "error",
		"-y",
		"-ss", "12.5",
		"-i", "input.webm",
		"-frames:v", "1",
		"-f", "image2",
		"out.jpg",
	}

	if !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("ScreenshotTime() = %#v, want %#v", []string(got), want)
	}
}

func TestScreenshotTimeSlowSeek(t *testing.T) {
	options := ScreenshotOptions{
		OutputPath: "out.jpg",
		OutputType: ScreenshotOutputTypeImage2,
		SlowSeek:   true,
	}

	got := ScreenshotTime("input.webm", 12.5, options)
	want := []string{
		"-v", "error",
		"-y",
		"-i", "input.webm",
		"-ss", "12.5",
		"-frames:v", "1",
		"-f", "image2",
		"out.jpg",
	}

	if !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("ScreenshotTime() = %#v, want %#v", []string(got), want)
	}
}

func TestScreenshotSquarePixels(t *testing.T) {
	for _, tt := range []struct {
		name   string
		width  int
		height int
		scale  string
	}{
		{"cover", 0, 0, ""},
		{"sprite width", 160, 0, ",scale=160:-2"},
		{"sprite height", 0, 160, ",scale=-2:160"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, slowSeek := range []bool{false, true} {
				options := ScreenshotOptions{
					OutputPath:        "out.jpg",
					OutputType:        ScreenshotOutputTypeImage2,
					Width:             tt.width,
					Height:            tt.height,
					SampleAspectRatio: 16.0 / 9,
					SlowSeek:          slowSeek,
				}
				args := ScreenshotTime("input.mp4", 2, options)
				want := "scale=trunc(iw*sar/2)*2:ih,setsar=1" + tt.scale
				assertScreenshotFilter(t, args, want)

				if tt.height == 0 {
					args = ScreenshotFrame("input.mp4", 3, options)
					assertScreenshotFilter(t, args, "select=eq(n\\,3),"+want)
				}

				// Explicit square pixels must produce exactly the unknown-SAR command.
				options.SampleAspectRatio = 1
				square := ScreenshotTime("input.mp4", 2, options)
				squareFrame := ScreenshotFrame("input.mp4", 3, options)
				options.SampleAspectRatio = 0
				if tt.scale != "" {
					assertScreenshotFilter(t, square, tt.scale[1:])
				} else {
					for _, arg := range square {
						if arg == "-vf" {
							t.Fatal("unscaled square-pixel screenshot gained a filter")
						}
					}
				}
				if !reflect.DeepEqual(square, ScreenshotTime("input.mp4", 2, options)) ||
					!reflect.DeepEqual(squareFrame, ScreenshotFrame("input.mp4", 3, options)) {
					t.Fatal("square pixels changed the unknown-SAR command")
				}
			}
		})
	}
}

func assertScreenshotFilter(t *testing.T, args []string, want string) {
	t.Helper()
	for i, arg := range args {
		if arg == "-vf" && i+1 < len(args) {
			if args[i+1] != want {
				t.Fatalf("filter = %q, want %q", args[i+1], want)
			}
			return
		}
	}
	t.Fatalf("missing filter in %v", args)
}
