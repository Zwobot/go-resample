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
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path"

// 	"time"
)

type namedFilter struct {
	Name string
	F    resample.Filter
}

var filters = [...]namedFilter{
	{"Box", resample.Box},
	{"Triangle", resample.Triangle},
	{"Lanczos3", resample.Lanczos3},
	{"Lanczos5", resample.Lanczos5},
	{"Lanczos12", resample.Lanczos12},
	{"Mitchell", resample.Mitchell},
	{"CatmullRom", resample.CatmullRom},
	{"BSpline", resample.BSpline}}

func drawProgress(win wde.Window, percent int) {
	black := color.RGBA{0, 0, 0, 255}
	white := color.RGBA{255, 255, 255, 255}
	screen := win.Screen()

	r := screen.Bounds()
	r = image.Rect(0, 0,
		r.Dx(), 20)

	draw.Draw(screen, r, &image.Uniform{black}, image.ZP, draw.Src)
	r2 := r
	r2.Min = r2.Min.Add(image.Pt(2, 2))
	r2.Max = r2.Max.Sub(image.Pt(2, 2))
	r2.Max.X = r2.Min.X + (r2.Dx()*percent)/100
	draw.Draw(screen, r2, &image.Uniform{white}, image.ZP, draw.Src)

	win.FlushImage(r)
}

func ResizeLoop(req <-chan image.Point, fchan <-chan namedFilter,
	win wde.Window, filename string, baseImage image.Image) {
	baseSize := baseImage.Bounds().Max
	var resizeChan <-chan resample.Step
	var newSize image.Point
	newFilter := namedFilter{"Box", resample.Box}
	for {
		select {
		case newFilter = <-fchan:
			win.SetTitle(fmt.Sprintf("%s %s %v %v", newFilter.Name, path.Base(filename), baseSize, newSize))
			log.Printf("%s %s %v %v", path.Base(filename), newFilter.Name, baseSize, newSize)
			resizeChan, _ = resample.ResizeToChannelWithFilter(
				newSize, baseImage,
				newFilter.F,
				resample.Reject,
				resample.Reject)

		case newSize = <-req:
			win.SetTitle(fmt.Sprintf("%s %s %v %v", newFilter.Name, path.Base(filename), baseSize, newSize))
			log.Printf("%s %s %v %v", path.Base(filename), newFilter.Name, baseSize, newSize)
			resizeChan, _ = resample.ResizeToChannelWithFilter(
				newSize, baseImage,
				newFilter.F,
				resample.Reject,
				resample.Reject)

		case step := <-resizeChan:
			if step.Done() {
				drawProgress(win, step.Percent())
				log.Printf("%s %v %v DONE (%d%%)", path.Base(filename),
					baseSize, newSize, step.Percent())
				screen := win.Screen()
				draw.Draw(screen, screen.Bounds(), step.Image(), image.ZP, draw.Src)
				win.FlushImage()
				resizeChan = nil
			} else {
				drawProgress(win, step.Percent())
				log.Printf("%s %v %v STEP (%d%%)", path.Base(filename),
					baseSize, newSize, step.Percent())
			}
		}
	}

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

	resizeChan := make(chan image.Point, 10)
	resizeChan <- image.Point{baseW, baseH}

	filterChan := make(chan namedFilter, 10)
	currentFilter := 0
	filterChan <- filters[currentFilter]

	go ResizeLoop(resizeChan, filterChan, win, *filename, baseImage)

	win.Show()

	events := win.EventChan()
	for event := range events {
		switch e := event.(type) {
		case wde.CloseEvent:
			return

		case wde.ResizeEvent:
			resizeChan <- image.Point{e.Width, e.Height}

		case wde.KeyUpEvent:
			currentFilter = (currentFilter + 1) % len(filters)
			filterChan <- filters[currentFilter]
		}
	}

}

func main() {
	go wdeMain()
	wde.Run()
}
