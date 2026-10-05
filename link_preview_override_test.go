package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func testPNGBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestBuildLinkPreviewOverride_LargeKeepsHQThumbnail(t *testing.T) {
	url, og := buildLinkPreviewOverride("veja https://a.com", &linkPreviewOverride{
		Url: "https://b.com", Title: "T", Description: "D", Thumbnail: testPNGBase64(t, 800, 400), Large: true,
	})
	if url != "https://b.com" || og.Title != "T" || og.Description != "D" {
		t.Fatalf("unexpected url/title/description: %q %q %q", url, og.Title, og.Description)
	}
	if len(og.ImageData) == 0 || len(og.HQImageData) == 0 || og.HQWidth == 0 || og.HQHeight == 0 {
		t.Fatal("expected inline and HQ thumbnails with dimensions")
	}
}

func TestBuildLinkPreviewOverride_SmallDropsHQThumbnail(t *testing.T) {
	_, og := buildLinkPreviewOverride("https://a.com", &linkPreviewOverride{Thumbnail: testPNGBase64(t, 800, 400), Large: false})
	if len(og.ImageData) == 0 || len(og.HQImageData) != 0 {
		t.Fatal("expected only the inline thumbnail")
	}
}

func TestBuildLinkPreviewOverride_FallsBackToURLInBodyAndIgnoresBadImage(t *testing.T) {
	url, og := buildLinkPreviewOverride("oi https://a.com/x", &linkPreviewOverride{Title: "T", Thumbnail: "não é base64!!"})
	if url != "https://a.com/x" || og.Title != "T" || len(og.ImageData) != 0 {
		t.Fatalf("unexpected result: %q %+v", url, og)
	}
}
