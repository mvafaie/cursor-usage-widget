package ui

import (
	"bytes"
	"os"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type faces struct {
	label  *text.GoTextFace
	pct    *text.GoTextFace
	remain *text.GoTextFace
	meta   *text.GoTextFace
	err    *text.GoTextFace
}

func loadFaces() (*faces, error) {
	regular, err := fontSource("/usr/share/fonts/truetype/ubuntu/Ubuntu-R.ttf", goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := fontSource("/usr/share/fonts/truetype/ubuntu/Ubuntu-B.ttf", gobold.TTF)
	if err != nil {
		return nil, err
	}
	return &faces{
		label:  &text.GoTextFace{Source: regular, Size: 12},
		pct:    &text.GoTextFace{Source: bold, Size: 20},
		remain: &text.GoTextFace{Source: bold, Size: 10},
		meta:   &text.GoTextFace{Source: regular, Size: 12},
		err:    &text.GoTextFace{Source: regular, Size: 14},
	}, nil
}

func fontSource(path string, fallback []byte) (*text.GoTextFaceSource, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return text.NewGoTextFaceSource(bytes.NewReader(b))
	}
	return text.NewGoTextFaceSource(bytes.NewReader(fallback))
}
