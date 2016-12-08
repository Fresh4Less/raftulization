package main

import(
	"fmt"
	"github.com/fresh4less/raftulization/ws2811"
)

type NeopixelDisplay struct {
	Count int
}

func NewNeopixelDisplay(gpioPin, ledCount, brightness int) *NeopixelDisplay {
	err := ws2811.Init(gpioPin, ledCount, brightness)
	if err != nil {
		panic(err)
	}
	display := NeopixelDisplay{ledCount}
	return &display
}

func (nd *NeopixelDisplay) Set(index int, color Color) {
	ws2811.SetLed(index, uint32(color))
}

func (nd *NeopixelDisplay) Show() {
	fmt.Printf("render\n")
	err := ws2811.Render()
	if err != nil {
		panic(err)
	}
}

type PixelDisplay struct {
	Display *NeopixelDisplay
	Offset, Width, Height int
	Wrap bool
	Colors [][]Color
}

//PixelDisplay defines a "view" into a NeoPixelDisplay that maps a 2d grid of colors onto the 1d led array
func NewPixelDisplay(display *NeopixelDisplay, offset, width, height int, wrap bool) *PixelDisplay {
	if offset < 0 || width < 0 || height < 0 || offset + width*height > display.Count {
		panic(fmt.Sprintf("NewPixelDisplay: invalid pixel dimensions (%v,%v,%v)", offset, width, height))
	}
	pd := PixelDisplay{display, offset, width, height, wrap, make([][]Color, height)}
	for i := 0; i < width; i++ {
		pd.Colors[i] = make([]Color, width)
	}

	return &pd
}

func (pd *PixelDisplay) Reset() {
	for i := 0; i < pd.Height; i++ {
		for j := 0; j < pd.Width; j++ {
			pd.Colors[i][j] = 0
		}
	}
}

func (pd *PixelDisplay) Set(row, col int, color Color) {
	if !pd.Wrap && (row < 0 || col < 0 || row >= pd.Height || col >= pd.Width) {
		panic(fmt.Sprintf("PixelDisplay: Set: tried to set (%v,%v) but the display has dimensions (%v,%v)", row, col, pd.Height, pd.Width))
	}
	pd.Colors[row % pd.Height][col % pd.Width] = color
}

// sets colors based on 2d array passed in
func (pd *PixelDisplay) SetArea(row, col int, colors [][]Color) {
	for i := 0; i < len(colors); i++ {
		for j := 0; j < len(colors[i]); j++ {
			pd.Set(row + i, col + j, colors[i][j])
		}
	}
}

func (pd *PixelDisplay) Draw() {
	for i := 0; i < pd.Height; i++ {
		for j := 0; j < pd.Width; j++ {
			pd.Display.Set(pd.Offset +  pd.Height*i + pd.Width, pd.Colors[i][j])
		}
	}
	pd.Display.Show()
}

type Color uint32

func (color Color) GetRed() uint32 {
	return uint32((color >> 16) & (2<<16 - 1))
}
func (color Color) GetGreen() uint32 {
	return uint32((color >> 8) & (2<<8 - 1))
}

func (color Color) GetBlue() uint32 {
	return uint32(color & (2<<8 - 1))
}

func (color Color) GetWhite() uint32 {
	return uint32((color >> 24) & (2<<24 - 1))
}

func MakeColor(red, green, blue uint32) Color {
	return Color((red<<16) | (green<<8) | blue)
}

func MakeColorHue(hue uint32) Color {
	for hue < 0 {
		hue += 255
	}
	for hue > 255 {
		hue -= 255
	}
	if hue < 85 {
		return MakeColor(hue * 3, 255 - hue * 3, 0)
	} else if hue < 170 {
		hue -= 85
		return MakeColor(255 - hue * 3, 0, hue * 3)
	} else {
		hue -= 170
		return MakeColor(0, hue * 3, 255 - hue * 3)
	}
}

func MakeColorRect(width, height int, color Color) [][]Color {
	colors := make([][]Color, height)
	for i := 0; i < height; i++ {
		colors[i] = make([]Color, width)
	}

	for i := 0; i < height; i++ {
		for j := 0; j < width; j++ {
				colors[i][j] = color
		}
	}

	return colors
}

func MakeColorPercentBar(width, height int, vertical bool, percent float32, fgColor, bgColor Color) [][]Color {
	cutoffIndex := int(percent*float32(width+1))
	colors := MakeColorRect(width, height, bgColor)

	for i := 0; i < len(colors); i++ {
		for j := 0; j < len(colors[i]); j++ {
			if (!vertical && j < cutoffIndex) ||
				(vertical && i < cutoffIndex) {
				colors[i][j] = fgColor
			}
		}
	}

	return colors
}

// Makes a 2x3 resolution number with the given colors
func MakeColorNumberChar(num int, fgColor, bgColor Color) [][]Color {
	colors := MakeColorRect(2,3, bgColor)
	for i := 0; i < len(colors); i++ {
		for j := 0; j < len(colors[i]); j++ {
			if NumberCharTemplates[num][i][j] {
				colors[i][j] = fgColor
			}
		}
	}

	return colors
}

var NumberCharTemplates [][][]bool = [][][]bool{
	//0
	[][]bool{
		[]bool{true,true},
		[]bool{true,true},
		[]bool{true,true},
	},
	//1
	[][]bool{
		[]bool{false,true},
		[]bool{false,true},
		[]bool{false,true},
	},
	//2
	[][]bool{
		[]bool{true,true},
		[]bool{false,true},
		[]bool{true,false},
	},
	//3
	[][]bool{
		[]bool{true,true},
		[]bool{false,true},
		[]bool{true,true},
	},
	//4
	[][]bool{
		[]bool{true,false},
		[]bool{true,true},
		[]bool{false,true},
	},
	//5
	[][]bool{
		[]bool{true,true},
		[]bool{true,false},
		[]bool{false,true},
	},
	//6
	[][]bool{
		[]bool{true,false},
		[]bool{true,true},
		[]bool{true,true},
	},
	//7
	[][]bool{
		[]bool{true,true},
		[]bool{false,true},
		[]bool{true,false},
	},
	//8
	[][]bool{
		[]bool{true,true},
		[]bool{false,false},
		[]bool{true,true},
	},
	//9
	[][]bool{
		[]bool{true,true},
		[]bool{true,true},
		[]bool{false,true},
	},
}
