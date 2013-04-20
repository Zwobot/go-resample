package main

import (
	"flag"
	"fmt"
	"github.com/Zwobot/go-resample/resample"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
	"runtime/pprof"
	"time"
)

func loadImage(filename string) image.Image {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Decode the image.
	pic, _, err := image.Decode(file)
	if err != nil {
		log.Fatal(err)
	}
	return pic
}

func saveImage(pic image.Image, filename string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	png.Encode(file, pic)
}

func sample(cpuprofile *string, src image.Image, dst image.Point) image.Image {
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	t0 := time.Now()
	fmt.Printf("resampling ...")
	out, _ := resample.Resize(dst, src)
	fmt.Printf("\rresampling done: %s\n", time.Now().Sub(t0))
	return out
}

func main() {
	var (
		InputFile  string
		OutputFile string
		W, H       int
	)
	var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	flag.StringVar(&InputFile, "image", "src/github.com/Zwobot/go-resample/gopher-logo.png", "image to resample")
	flag.IntVar(&W, "w", 1000, "new width")
	flag.IntVar(&H, "h", 560, "new height")
	flag.StringVar(&OutputFile, "o", "out.png", "output")
	flag.Parse()

	fmt.Printf("loading %s ...", InputFile)
	src := loadImage(InputFile)
	fmt.Printf("\rloaded %s %v\n", InputFile, src.Bounds().Max)

	dst := sample(cpuprofile, src, image.Pt(W, H))
	fmt.Printf("saving %s %v...", OutputFile, dst.Bounds().Max)
	saveImage(dst, OutputFile)
	fmt.Printf("\rsaved %s %v     \n", OutputFile, dst.Bounds().Max)
}
