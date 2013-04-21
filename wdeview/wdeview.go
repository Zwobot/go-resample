// A go.wde based image viewer.
package main

import (
	_ "code.google.com/p/draw2d/draw2d"
	"flag"
	"fmt"
	"github.com/Zwobot/go-resample/resample"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path"
	"time"
)

func ResizeLoop(win wde.Window, filename string, baseImage image.Image) chan image.Point {
	req := make(chan image.Point, 10)

	skipRequests := func(pt image.Point, delay time.Duration) image.Point {
		for {
			select {
			case pt = <-req:
			case <-time.After(delay):
				return pt
			}
		}
		return pt
	}

	go func() {
		baseSize := baseImage.Bounds().Max
		for newSize := range req {
			newSize = skipRequests(newSize, 10*time.Microsecond)

			win.SetSize(newSize.X, newSize.Y)
			win.SetTitle(fmt.Sprintf("%s %v %v", path.Base(filename), baseSize, newSize))

			resizeJob, _ := resample.ResizeToChannel(newSize, baseImage)
			var resizeStep resample.Step
			for resizeStep.Image == nil {
				resizeStep = <-resizeJob
				nextNewSize := skipRequests(newSize, 10*time.Microsecond)
				if nextNewSize != newSize {
					close(resizeJob)
					newSize = nextNewSize
					resizeJob, _ = resample.ResizeToChannel(newSize, baseImage)

					win.SetSize(newSize.X, newSize.Y)
					win.SetTitle(fmt.Sprintf("%s %v %v", path.Base(filename), baseSize, newSize))
				}
			}
			screen := win.Screen()
			draw.Draw(screen, screen.Bounds(), resizeStep.Image, image.ZP, draw.Src)
			win.FlushImage()
		}
	}()
	return req
}

func wdeMain() {
	defer wde.Stop()

	filename := flag.String("image", "src/github.com/Zwobot/go-resample/gopher-logo.png", "image to view")
	flag.Parse()
	if filename == nil || *filename == "" {
		flag.PrintDefaults()
		return
	}

	file, err := os.Open(*filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Decode the image.
	baseImage, _, err := image.Decode(file)
	if err != nil {
		log.Fatal(err)
	}

	baseW := baseImage.Bounds().Max.X
	baseH := baseImage.Bounds().Max.Y

	win, err := wde.NewWindow(baseW, baseH)
	if err != nil {
		log.Fatalf("%s", err.Error())
	}

	resizeChan := ResizeLoop(win, *filename, baseImage)

	resizeChan <- image.Point{baseW, baseH}
	win.Show()

	events := win.EventChan()
	for event := range events {
		switch e := event.(type) {
		case wde.CloseEvent:
			return

		case wde.ResizeEvent:
			resizeChan <- image.Point{e.Width, e.Height}

		}
	}

}

func main() {
	go wdeMain()
	wde.Run()
}
