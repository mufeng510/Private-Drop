package upload

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
)

// imageDimensions reads image dimensions when possible. Failures are ignored.
func imageDimensions(path, mime string) (int, int) {
	if !strings.HasPrefix(strings.ToLower(mime), "image/") {
		return 0, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
