package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/draw"

	"github.com/mrv/cursor-usage-widget/internal/cursor"
	"github.com/mrv/cursor-usage-widget/internal/ui"
)

//go:embed assets/icon.png
var iconPNG []byte

func main() {
	once := flag.Bool("once", false, "print usage JSON and exit (no widget)")
	poll := flag.Duration("poll", 90*time.Second, "how often to refresh usage")
	flag.Parse()

	client := cursor.NewClient()

	if *once {
		snap := client.Fetch(context.Background())
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			log.Fatal(err)
		}
		if snap.Err != "" {
			os.Exit(1)
		}
		return
	}

	src := decodeIcon()
	w, err := ui.New(client, *poll, src)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowTitle("cursor remaining")
	if src != nil {
		ebiten.SetWindowIcon(iconSizes(src))
	}
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	ebiten.SetWindowMousePassthrough(true)
	ww, wh := ui.WindowSize()
	ebiten.SetWindowSize(ww, wh)
	ebiten.SetTPS(30)
	ui.RestorePosition()

	op := &ebiten.RunGameOptions{
		ScreenTransparent: true,
		SkipTaskbar:       true,
		InitUnfocused:     true,
		X11ClassName:      "cursor-remaining",
		X11InstanceName:   "cursor remaining",
	}
	if err := ebiten.RunGameWithOptions(w, op); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func decodeIcon() image.Image {
	src, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return nil
	}
	return src
}

func iconSizes(src image.Image) []image.Image {
	icons := []image.Image{src}
	for _, n := range []int{48, 32, 16} {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		icons = append(icons, dst)
	}
	return icons
}
