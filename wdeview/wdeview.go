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

// 	"time"
)

func ResizeLoop(win wde.Window, filename string, baseImage image.Image) chan image.Point {
	req := make(chan image.Point, 10)

	go func() {
		baseSize := baseImage.Bounds().Max
		var resizeChan <-chan resample.Step
		var newSize image.Point
		for {
			select {
			case newSize = <-req:
				win.SetTitle(fmt.Sprintf("%s %v %v", path.Base(filename), baseSize, newSize))
				log.Printf("%s %v %v", path.Base(filename), baseSize, newSize)
				resizeChan, _ = resample.ResizeToChannelWithFilter(
					newSize, baseImage,
					resample.BSpline,
					resample.Reject,
					resample.Reject)

			case step := <-resizeChan:
				if step.Done() {
					log.Printf("%s %v %v DONE (%d%%)", path.Base(filename),
						baseSize, newSize, step.Percent())
					screen := win.Screen()
					draw.Draw(screen, screen.Bounds(), step.Image(), image.ZP, draw.Src)
					win.FlushImage()
					resizeChan = nil
				} else {
					log.Printf("%s %v %v STEP (%d%%)", path.Base(filename),
						baseSize, newSize, step.Percent())
				}
			}
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
