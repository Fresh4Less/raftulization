package main

import (
	"fmt"
	"github.com/fresh4less/raftulization/ws2811"
	"time"
)

type PixelDisplayView struct {
	Display               PixelDisplay
	Offset, Width, Height int
	Brightness float32
	Wrap                  bool
	Colors                ColorFrame
}

//PixelDisplayView defines a "view" into a NeoPixelDisplayView that maps a 2d grid of colors onto the 1d led array
func NewPixelDisplayView(display PixelDisplay, offset, width, height int, brightness float32, wrap bool) *PixelDisplayView {
	if offset < 0 || width < 0 || height < 0 || offset+width*height > display.Count() {
		panic(fmt.Sprintf("NewPixelDisplayView: invalid pixel dimensions (%v,%v,%v)", offset, width, height))
	}
	pd := PixelDisplayView{display, offset, width, height, brightness, wrap, MakeColorFrame(width, height, MakeColor(0,0,0))}

	return &pd
}

// copy frame from top-left corner to bottom right. Will error if frame is smaller than PixelDisplayView
func (pd *PixelDisplayView) SetFrame(frame ColorFrame) {
	for i := 0; i < pd.Height; i++ {
		for j := 0; j < pd.Width; j++ {
			pd.Colors[i][j] = MakeColor(
				uint32(float32(frame[i][j].GetRed())*pd.Brightness),
				uint32(float32(frame[i][j].GetGreen())*pd.Brightness),
				uint32(float32(frame[i][j].GetBlue())*pd.Brightness))
		}
	}
}

func (pd *PixelDisplayView) Draw() {
	for i := 0; i < pd.Height; i++ {
		for j := 0; j < pd.Width; j++ {
			pd.Display.Set(pd.Offset+pd.Height*i+j, pd.Colors[i][j])
			//if pd.Colors[i][j] == 0 {
				//fmt.Printf("*")
			//} else if pd.Colors[i][j] == MakeColor(255,0,0) {
				//fmt.Printf("x")
			//} else {
				//fmt.Printf("0")
			//}
		}
		//fmt.Printf("\n")
	}
	//fmt.Printf("\n")
	pd.Display.Show()
}

// blocks
func (pd *PixelDisplayView) DrawAnimation(frames []ColorFrame, fps float32) {
	for _, frame := range frames {
		pd.SetFrame(frame)
		pd.Draw()
		time.Sleep(time.Second / (time.Duration(fps)*time.Second))
	}
}

/********** Color **********/
type Color uint32

func (color Color) GetRed() uint32 {
	return uint32((color >> 8) & (2<<8 - 1))
}

func (color Color) GetGreen() uint32 {
	return uint32((color >> 16) & (2<<8 - 1))
}

func (color Color) GetBlue() uint32 {
	return uint32(color & (2<<8 - 1))
}

//func (color Color) GetWhite() uint32 {
	//return uint32((color >> 24) & (2<<8 - 1))
//}

func MakeColor(red, green, blue uint32) Color {
	return Color((green << 16) | (red << 8) | blue)
}

/********** ColorFrame **********/

type ColorOverflowMode int
const(
	Error ColorOverflowMode = iota
	Clip
	Wrap
)

type ColorFrame [][]Color

func MakeColorFrame(width, height int, color Color) ColorFrame {
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

func (c ColorFrame) Set(x, y int, color Color, overflowMode ColorOverflowMode) {
	if y < 0 || x < 0 || y >= len(c) || x >= len(c[y]) {
		switch overflowMode {
			case Error:
				panic(fmt.Sprintf("ColorFrame.Set: tried to set (%v,%v) but the frame has dimensions (%v,%v)",
					x, y, len(c[y]), len(c)))
			case Clip:
				return
			case Wrap:
				y = y % len(c)
				x = x % len(c[y])
		}
	}
	c[y][x] = color
}

func (c ColorFrame) SetAll(color Color) {
	for i := 0; i < len(c); i++ {
		for j := 0; j < len(c[i]); j++ {
			c[i][j] = color
		}
	}
}

func (c ColorFrame) SetRect(x, y int, source ColorFrame, overflowMode ColorOverflowMode) {
	for i := 0; i < len(source); i++ {
		for j := 0; j < len(source[i]); j++ {
			c.Set(x+j, y+i, source[i][j], overflowMode)
		}
	}
}

/********** PixelDisplay **********/

type PixelDisplay interface {
	Set(index int, color Color)
	Show()
	Count() int
}

type FakeDisplay struct {
	count int
}

func (fd *FakeDisplay) Set(index int, color Color) {
	//fmt.Printf("set %v, %v\n", index, color)
}
func (fd *FakeDisplay) Show() {
}

func (fd *FakeDisplay) Count() int {
	return fd.count
}

type NeopixelDisplay struct {
	count int
}

func NewNeopixelDisplay(gpioPin, ledCount, brightness int) *NeopixelDisplay {
	err := ws2811.Init(gpioPin, ledCount, brightness)
	if err != nil {
		panic(err)
	}
	display := NeopixelDisplay{ledCount}
	return &display
}

func (nd *NeopixelDisplay) Count() int {
	return nd.count
}

func (nd *NeopixelDisplay) Set(index int, color Color) {
	//fmt.Printf("set %v\n", index)
	ws2811.SetLed(index, uint32(color))
}

func (nd *NeopixelDisplay) Show() {
	//fmt.Printf("render\n")
	err := ws2811.Render()
	if err != nil {
		panic(err)
	}
}

/********** MultiFrameView ***********/

// multiplexes mutliple ColorFrames, allowing selection and cycling of frames with transition animations
// currentFrame is always drawn, if transitioning==true the next frame will be drawn as well
type MultiFrameView struct {
	display *PixelDisplayView
	currentFrame int
	transitioning bool
	transitionIndex int
	frames []struct {
		frame *ColorFrame
		x, y int
		duration time.Duration
		transition FrameTransition
	}
}

func MakeMultiFrameView(display *PixelDisplayView) *MultiFrameView {
	return &MultiFrameView{display, 0, false, 0, nil}
}

type FrameTransition int
const(
	None = iota
	Slide
)

func (mfv *MultiFrameView) CycleFrames(frames []*ColorFrame, durations []time.Duration, transitions []FrameTransition) {
	mfv.frames = make([]struct {
		frame *ColorFrame;
		x, y int;
		duration time.Duration;
		transition FrameTransition}, len(frames))

	for i := range frames {
		mfv.frames[i].frame = frames[i]
		mfv.frames[i].duration = durations[i]
		mfv.frames[i].transition = transitions[i]
	}

	mfv.beginTransition(0, 0, None)
}


func (mfv *MultiFrameView) UpdateFrame(frame *ColorFrame) {
	//redraw if visible
	if (mfv.frames[mfv.currentFrame].frame == frame) ||
		(mfv.transitioning && mfv.frames[(mfv.currentFrame + 1) % len(mfv.frames)].frame == frame) {
			mfv.draw()
	}
}


func (mfv *MultiFrameView) beginTransition(frameIndex int, duration time.Duration, transition FrameTransition) {
	fmt.Printf("begin transition %v\n", frameIndex)
	mfv.transitionIndex++
	transitionIndex := mfv.transitionIndex
	mfv.currentFrame = frameIndex
	mfv.frames[frameIndex].x = 0
	mfv.frames[frameIndex].y = 0

	switch transition {
		case None:
		mfv.transitioning = false
		case Slide:
			nextFrameIndex := (frameIndex + 1) % len(mfv.frames)
			//for now just transition left to right
			mfv.frames[nextFrameIndex].x = mfv.display.Width
			mfv.frames[nextFrameIndex].y = 0
			mfv.transitioning = true
			go func() {
				for i := 0; i < mfv.display.Width; i++ {
					time.Sleep(time.Duration(int(duration/time.Millisecond) / mfv.display.Width)*time.Millisecond)
					if mfv.transitionIndex != transitionIndex {
						//someone else started a transition while we were sleeping--cancel the rest of this transition
						return
					}

					mfv.frames[frameIndex].x--
					mfv.frames[nextFrameIndex].x--
					mfv.draw()
				}

				mfv.currentFrame = nextFrameIndex
				mfv.transitioning = false
				mfv.beginTransition(mfv.currentFrame, mfv.frames[mfv.currentFrame].duration, mfv.frames[mfv.currentFrame].transition)
			}()

	}
	mfv.draw()
}

func (mfv *MultiFrameView) draw() {
	frame := mfv.frames[mfv.currentFrame]
	mfv.display.Colors.SetRect(frame.x, frame.y, *frame.frame, Clip)
	if mfv.transitioning {
		//draw next frame
		nextFrame := mfv.frames[(mfv.currentFrame + 1) % len(mfv.frames)]
		mfv.display.Colors.SetRect(nextFrame.x, nextFrame.y, *nextFrame.frame, Clip)
	}
	fmt.Printf("draw %v\n", mfv.display.Colors)
	mfv.display.Draw()
}

/********** Color Helper Constructors **********/
func MakeColorHue(hue uint32) Color {
	for hue < 0 {
		hue += 255
	}
	for hue > 255 {
		hue -= 255
	}
	if hue < 85 {
		return MakeColor(hue*3, 255-hue*3, 0)
	} else if hue < 170 {
		hue -= 85
		return MakeColor(255-hue*3, 0, hue*3)
	} else {
		hue -= 170
		return MakeColor(0, hue*3, 255-hue*3)
	}
}

func MakeColorPercentBar(width, height int, vertical bool, percent float32, fgColor, bgColor Color) ColorFrame {
	cutoffIndex := int(percent * float32(width+1))
	colors := MakeColorFrame(width, height, bgColor)

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
func MakeColorNumberChar(num int, fgColor, bgColor Color) ColorFrame {
	colors := MakeColorFrame(2, 3, bgColor)
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
		[]bool{true, true},
		[]bool{true, true},
		[]bool{true, true},
	},
	//1
	[][]bool{
		[]bool{false, true},
		[]bool{false, true},
		[]bool{false, true},
	},
	//2
	[][]bool{
		[]bool{true, true},
		[]bool{false, true},
		[]bool{true, false},
	},
	//3
	[][]bool{
		[]bool{true, true},
		[]bool{false, true},
		[]bool{true, true},
	},
	//4
	[][]bool{
		[]bool{true, false},
		[]bool{true, true},
		[]bool{false, true},
	},
	//5
	[][]bool{
		[]bool{true, true},
		[]bool{true, false},
		[]bool{false, true},
	},
	//6
	[][]bool{
		[]bool{true, false},
		[]bool{true, true},
		[]bool{true, true},
	},
	//7
	[][]bool{
		[]bool{true, true},
		[]bool{false, true},
		[]bool{true, false},
	},
	//8
	[][]bool{
		[]bool{true, true},
		[]bool{false, false},
		[]bool{true, true},
	},
	//9
	[][]bool{
		[]bool{true, true},
		[]bool{true, true},
		[]bool{false, true},
	},
}
