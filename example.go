package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	_ "image/jpeg"
	"resample"
	"log"
	"os"
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

func main() {
	var (
		InputFile  string
		OutputFile string
		W, H       int
	)
	flag.StringVar(&InputFile, "img", "", "image to resample")
	flag.IntVar(&W, "w", 100, "new width")
	flag.IntVar(&H, "h", 100, "new height")
	flag.StringVar(&OutputFile, "o", "", "output")
	flag.Parse()

	src := loadImage(InputFile)
	fmt.Printf("%s %+v\n", InputFile, src.Bounds())

	src_nrgba64 := image.NewNRGBA64(src.Bounds())
	draw.Draw(src_nrgba64, src_nrgba64.Bounds(),
		src, image.ZP, draw.Src)

	dst := image.NewNRGBA64(image.Rect(0, 0, W, H))
	fmt.Printf("%s %+v\n", OutputFile, dst.Bounds())

	resample.ResampleNRGBA64(dst, src_nrgba64, resample.Lanczos12, resample.Clamp, resample.Clamp)

	saveImage(dst, OutputFile)

}
