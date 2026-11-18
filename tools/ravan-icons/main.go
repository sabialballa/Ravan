// Command ravan-icons renders Ravan's own mark without upstream logo assets.
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	for name, size := range map[string]int{"icon-192.png": 192, "icon-512.png": 512, "icon-maskable-512.png": 512, "apple-touch-icon.png": 180} {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				img.Set(x, y, color.RGBA{12, 25, 39, 255})
				u, v := 128*x/size, 128*y/size
				stem := u >= 32 && u <= 44 && v >= 28 && v <= 100
				bowl := u >= 44 && u <= 88 && ((v >= 28 && v <= 40) || (v >= 56 && v <= 68) || (u >= 76 && v >= 40 && v <= 56))
				leg := v >= 66 && v <= 100 && u >= 48+(v-66)/2 && u <= 63+(v-66)/2
				if stem || bowl || leg {
					img.Set(x, y, color.RGBA{240, 156, 56, 255})
				}
			}
		}
		f, err := os.Create(filepath.Join("internal/webui/assets/icons", name))
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}
}
