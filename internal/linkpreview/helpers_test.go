package linkpreview_test

import (
	"testing"

	"private-drop/internal/linkpreview"
)

func TestExtractURLs(t *testing.T) {
	text := "see https://example.com/a?b=1 and http://example.org/x, plus https://example.com/a?b=1"
	got := linkpreview.ExtractURLs(text, 5)
	if len(got) != 2 {
		t.Fatalf("ExtractURLs = %#v, want 2 unique urls", got)
	}
	if got[0] != "https://example.com/a?b=1" || got[1] != "http://example.org/x" {
		t.Fatalf("unexpected urls: %#v", got)
	}
	if len(linkpreview.ExtractURLs(text, 1)) != 1 {
		t.Fatal("max parameter must bound the result count")
	}
}

func TestValidateImageURLSSRF(t *testing.T) {
	for _, u := range []string{
		"http://127.0.0.1/x.png",
		"http://localhost/x.png",
		"http://169.254.169.254/latest/meta-data",
		"file:///etc/passwd",
		"not a url",
	} {
		if err := linkpreview.ValidateImageURL(u); err == nil {
			t.Errorf("ValidateImageURL(%q) should be rejected", u)
		}
	}
	if err := linkpreview.ValidateImageURL(""); err != nil {
		t.Errorf("empty image URL should be allowed: %v", err)
	}
}
