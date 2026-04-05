package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math/rand"
	"time"

	ws "github.com/mxcu/go-waveshare-4.2-epd"
)

func main() {
	fmt.Println("Truchet Tiles & Clock Demo")

	epd, err := ws.New()
	if err != nil {
		log.Fatalf("Error initializing EPD: %v", err)
	}
	defer epd.Close()

	// 1. Background: Truchet Tiles (Full Refresh)
	fmt.Println("Creating Truchet background...")
	background := makeTruchetTiles2(ws.Width, ws.Height, 12)
	epd.DisplayImage(background)
	fmt.Println("Background displayed.")

	// 2. Render Clock with 4 grayscales
	clockImg := makeClockImage(ws.Width, ws.Height, time.Now(), background)
	epd.DisplayImage(clockImg)
	fmt.Println("Demo finished. Exiting...")
}

// makeTruchetTiles creates a classic Truchet pattern (diagonals).
func makeTruchetTiles2(w, h, tileSize int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Gray{Y: 0xFF}), image.Point{}, draw.Src)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for y := 0; y < h; y += tileSize {
		for x := 0; x < w; x += tileSize {
			// Randomly choose gray levels
			bgGray := color.Gray{Y: 0xAA} // Light Gray
			fgGray := color.Gray{Y: 0x55} // Dark Gray

			if r.Intn(2) == 0 {
				fgGray = color.Gray{Y: 0x00} // Black
			}

			// Fill tile background
			for i := range tileSize {
				for j := range tileSize {
					img.Set(x+i, y+j, bgGray)
				}
			}

			if r.Intn(2) == 0 {
				// Diagonal: /
				for i := range tileSize {
					img.Set(x+tileSize-1-i, y+i, fgGray)
					if i > 0 {
						img.Set(x+tileSize-i, y+i, fgGray)
					}
				}
			} else {
				// Diagonal: \
				for i := range tileSize {
					img.Set(x+i, y+i, fgGray)
					if i > 0 {
						img.Set(x+i-1, y+i, fgGray)
					}
				}
			}
		}
	}
	return img
}

// makeClockImage creates an image containing current date and time on top of the background.
func makeClockImage(w, h int, t time.Time, bg image.Image) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Draw background
	draw.Draw(img, img.Bounds(), bg, image.Point{}, draw.Src)

	dateStr := t.Format("2006-01-02")
	timeStr := t.Format("15:04:05")

	// Box for the time
	boxW, boxH := 300, 120
	boxX, boxY := (w-boxW)/2, (w-boxH)/2
	boxRect := image.Rect(boxX, boxY, boxX+boxW, boxY+boxH)
	draw.Draw(img, boxRect, image.NewUniform(color.Gray{Y: 0xFF}), image.Point{}, draw.Src)

	// Box border
	border := image.Rect(boxX-2, boxY-2, boxX+boxW+2, boxY+boxH+2)
	for i := border.Min.X; i < border.Max.X; i++ {
		img.Set(i, border.Min.Y, color.Black)
		img.Set(i, border.Max.Y-1, color.Black)
	}
	for i := border.Min.Y; i < border.Max.Y; i++ {
		img.Set(border.Min.X, i, color.Black)
		img.Set(border.Max.X-1, i, color.Black)
	}

	// Date (top)
	dateScale := 4
	dateW := len(dateStr) * (3*dateScale + dateScale)
	drawText(img, boxX+(boxW-dateW)/2, boxY+20, dateStr, dateScale)

	// Time (bottom)
	timeScale := 6
	timeW := len(timeStr) * (3*timeScale + timeScale)
	drawText(img, boxX+(boxW-timeW)/2, boxY+60, timeStr, timeScale)

	return img
}

// drawText renders text using a simple 3x5 bitmap font and scaling.
func drawText(img *image.RGBA, x, y int, s string, scale int) {
	for i, char := range s {
		drawChar(img, x+i*(3*scale+scale), y, char, scale)
	}
}

var font = map[rune][]string{
	'0': {"###", "# #", "# #", "# #", "###"},
	'1': {"  #", "  #", "  #", "  #", "  #"},
	'2': {"###", "  #", "###", "#  ", "###"},
	'3': {"###", "  #", "###", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"},
	'5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"},
	'7': {"###", "  #", "  #", "  #", "  #"},
	'8': {"###", "# #", "###", "# #", "###"},
	'9': {"###", "# #", "###", "  #", "###"},
	':': {"   ", " # ", "   ", " # ", "   "},
	'-': {"   ", "   ", "###", "   ", "   "},
	' ': {"   ", "   ", "   ", "   ", "   "},
}

func drawChar(img *image.RGBA, x, y int, char rune, scale int) {
	bitmap, ok := font[char]
	if !ok {
		return
	}
	for row, line := range bitmap {
		for col, bit := range line {
			if bit == '#' {
				rect := image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale)
				draw.Draw(img, rect, image.NewUniform(color.Black), image.Point{}, draw.Src)
			}
		}
	}
}
