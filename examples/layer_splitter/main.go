package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"github.com/gonutz/xcf"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("please provide an XCF as the first parameter")
		return
	}

	canvas, err := xcf.LoadFromFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	for _, layer := range canvas.Layers {
		saveImageAsPng(layer, "./"+layer.Name+"_layer.png")
	}

	saveImageAsPng(composeLayers(canvas), "./composed_layer.png")
}

func saveImageAsPng(img image.Image, path string) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	png.Encode(file, img)
}

func composeLayers(canvas xcf.Canvas) image.Image {
	// create an image of the canvas' size
	img := image.NewRGBA(image.Rect(0, 0, int(canvas.Width), int(canvas.Height)))

	// fill it with a transparent background
	draw.Draw(img, img.Bounds(), image.Transparent, image.ZP, draw.Src)

	// starting with the bottom-most layer, draw all visible layers over the
	// image, with their respective opacity
	for i := len(canvas.Layers) - 1; i >= 0; i-- {
		layer := canvas.Layers[i]
		if layer.Visible {
			opacity := image.NewUniform(color.RGBA{0, 0, 0, layer.Opacity})
			draw.DrawMask(img, layer.Bounds(), layer, layer.Bounds().Min, opacity, image.ZP, draw.Over)
		}
	}
	return img
}
