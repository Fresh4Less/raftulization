package main

import (
	"fmt"
	"github.com/fresh4less/raftulization/ws2811"
	"errors"
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
			//if pd.Width == 30 {
				//if pd.Colors[i][j] == 0 {
					//fmt.Printf("*")
				//} else if pd.Colors[i][j] == MakeColor(255,255,255) {
					//fmt.Printf("x")
				//} else {
					//fmt.Printf("0")
				//}
			//}
		}
		//if pd.Width == 30 {
			//fmt.Printf("\n")
		//}
	}
	//if pd.Width == 30 {
		//fmt.Printf("\n")
	//}
	pd.Display.Show()
}

// blocks
func (pd *PixelDisplayView) DrawAnimation(frames []ColorFrame, fps float32) {
	for _, frame := range frames {
		pd.SetFrame(frame)
		pd.Draw()
		time.Sleep(time.Duration(1000.0/fps)*time.Millisecond)
	}
}

/********** Color **********/
type Color uint32

func (color Color) GetRed() uint32 {
	return uint32((color >> 8) & (1<<8 - 1))
}

func (color Color) GetGreen() uint32 {
	return uint32((color >> 16) & (1<<8 - 1))
}

func (color Color) GetBlue() uint32 {
	return uint32(color & (1<<8 - 1))
}

//func (color Color) GetWhite() uint32 {
	//return uint32((color >> 24) & (2<<8 - 1))
//}

func MakeColor(red, green, blue uint32) Color {
	return Color((green << 16) | (red << 8) | blue)
}

func (c Color) Add(c2 Color) Color {
	return MakeColor(
		MinUint32(255, c.GetRed() + c2.GetRed()),
		MinUint32(255, c.GetGreen() + c2.GetGreen()),
		MinUint32(255, c.GetBlue() + c2.GetBlue()))
}

func MinUint32(a, b uint32) uint32 {
	if a < b {
		return a
	} else {
		return b
	}
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

//returns x, y, errorCode, error
func (c ColorFrame) calcOverflowPosition(x, y int, overflowMode ColorOverflowMode) (int, int, int, error) {
	if y < 0 || x < 0 || y >= len(c) || x >= len(c[y]) {
		switch overflowMode {
			case Error:
				width := "?"
				if y >= 0 && y < len(c) {
					width = fmt.Sprintf("%v", len(c))
				}
				return 0, 0, 1, errors.New(fmt.Sprintf("ColorFrame.Set: tried to set (%v,%v) but the frame has dimensions (%v,%v)",
					x, y, width, len(c)))
			case Clip:
				return 0, 0, 2, nil
			case Wrap:
				return x % len(c[y]), y % len(c), 0, nil
		}
	}
	return x, y, 0, nil
}

//returns 0,0,0 if set to clip mode
func (c ColorFrame) Get(x, y int, overflowMode ColorOverflowMode) Color {
	x, y, errorCode, err := c.calcOverflowPosition(x, y, overflowMode)
	if errorCode == 1 {
		panic(err)
	} else if errorCode == 2 {
		return MakeColor(0,0,0)
	}

	return c[y][x]
}
func (c ColorFrame) Set(x, y int, color Color, overflowMode ColorOverflowMode) {
	x, y, errorCode, err := c.calcOverflowPosition(x, y, overflowMode)
	if errorCode == 1 {
		panic(err)
	} else if errorCode == 2 {
		// do nothing
		return
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

type ColorCombineMode int

const(
	OverwriteAll ColorCombineMode = iota
	Overwrite // 0s don't overwrite colors
	Add
	SetWhite

)

func (c ColorFrame) CombineRect(x, y int, source ColorFrame, combineMode ColorCombineMode, overflowMode ColorOverflowMode) {
	for i := 0; i < len(source); i++ {
		for j := 0; j < len(source[i]); j++ {
			switch combineMode {
			case OverwriteAll:
				c.Set(x+j, y+i, source[i][j], overflowMode)
			case Overwrite:
				if source[i][j] != MakeColor(0,0,0) {
					c.Set(x+j, y+i, source[i][j], overflowMode)
				}
			case Add:
				c.Set(x+j, y+i, c.Get(x+j, y+i, overflowMode).Add(source[i][j]), overflowMode)
			case SetWhite:
				if source[i][j] != MakeColor(0,0,0) {
					if c.Get(x+j, y+i, overflowMode) != MakeColor(0,0,0) {
						c.Set(x+j, y+i, MakeColor(255,255,255), overflowMode)
					} else {
						c.Set(x+j, y+i, source[i][j], overflowMode)
					}
				}
			}
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
	cooloffTimer *time.Timer
	timerRunning bool
	renderAfterTimer bool
}

const DisplayCooloffTime = 5*time.Millisecond

//interface to the neopixel API
func NewNeopixelDisplay(gpioPin, ledCount, brightness int) *NeopixelDisplay {
	err := ws2811.Init(gpioPin, ledCount, brightness)
	if err != nil {
		panic(err)
	}
	display := NeopixelDisplay{ledCount, time.NewTimer(DisplayCooloffTime), false, false}
	//go func() {
		//for true {
			//display.render()
			//time.Sleep(5*time.Millisecond)
		//}
	//}()
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
	if nd.timerRunning {
		if !nd.renderAfterTimer {
			nd.renderAfterTimer = true
			go func() {
				<-nd.cooloffTimer.C
				nd.render()
			}()
		}
	} else {
	nd.render()
	}
}

func (nd *NeopixelDisplay) render() {
	//fmt.Printf("render\n")
	err := ws2811.Render()
	if err != nil {
		panic(err)
	}

	//set timer
	nd.cooloffTimer.Reset(DisplayCooloffTime)
	nd.timerRunning = true
	nd.renderAfterTimer = false
}

/********** MultiAnimationView **********/
// draws multiple animations on top of each other
type AnimationData struct {
	animation []ColorFrame
	frameIndex int
}

type MultiAnimationView struct {
	display *PixelDisplayView
	combineMode ColorCombineMode
	overflowMode ColorOverflowMode
	animations []*AnimationData
	bgColor Color
}

func NewMultiAnimationView(display *PixelDisplayView, combineMode ColorCombineMode, overflowMode ColorOverflowMode) *MultiAnimationView {
	return &MultiAnimationView{display, combineMode, overflowMode, make([]*AnimationData, 0), MakeColor(0,0,0)}
}

func (mav *MultiAnimationView) SetBackgroundColor(color Color) {
	mav.bgColor = color
	mav.draw()
}

func (mav *MultiAnimationView) AddAnimation(animation []ColorFrame, fps float32) {
	animationData := AnimationData{animation, 0}
	mav.animations = append(mav.animations, &animationData)
	mav.draw()
	go func() {
		for i := 1; i < len(animation); i++ {
			time.Sleep(time.Duration(1000.0/fps)*time.Millisecond)
			animationData.frameIndex = i
			mav.draw()
		}
		//remove animation from draw list
		for i, anim := range mav.animations {
			if anim == &animationData {
				mav.animations = append(mav.animations[:i], mav.animations[i+1:]...)
			}
		}
	}()
}

func (mav *MultiAnimationView) draw() {
	fullFrame := MakeColorFrame(mav.display.Width, mav.display.Height, MakeColor(0,0,0))
	for _, animationData := range mav.animations {
		fullFrame.CombineRect(0, 0, animationData.animation[animationData.frameIndex], mav.combineMode, mav.overflowMode)
	}
	finalFrame := MakeColorFrame(mav.display.Width, mav.display.Height, mav.bgColor)
	finalFrame.CombineRect(0,0, fullFrame, Overwrite, Error)
	mav.display.SetFrame(finalFrame)
	mav.display.Draw()
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

func NewMultiFrameView(display *PixelDisplayView) *MultiFrameView {
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

	mfv.beginTransition(0, None)
}


func (mfv *MultiFrameView) UpdateFrame(frame *ColorFrame) {
	//redraw if visible
	//if (mfv.frames[mfv.currentFrame].frame == frame) ||
		//(mfv.transitioning && mfv.frames[(mfv.currentFrame + 1) % len(mfv.frames)].frame == frame) {
			mfv.draw()
	//}
}


func (mfv *MultiFrameView) beginTransition(frameIndex int, transition FrameTransition) {
	mfv.transitionIndex++
	transitionIndex := mfv.transitionIndex

	beginIndex := (((frameIndex-1)%len(mfv.frames))+ len(mfv.frames)) % len(mfv.frames) //double modulus prevents negative index

	mfv.frames[beginIndex].x = 0
	mfv.frames[beginIndex].y = 0

	mfv.transitioning = true
	// async transition
	go func() {
		switch transition {
		case None:
			mfv.frames[frameIndex].x = 0
		case Slide:
			//for now just transition left to right
			mfv.frames[frameIndex].x = mfv.display.Width
			for i := 0; i < mfv.display.Width; i++ {
				mfv.frames[beginIndex].x--
				mfv.frames[frameIndex].x--
				mfv.draw()

				//TODO: don't hardcode transition duration
				time.Sleep(time.Duration(500/8)*time.Millisecond)
				if mfv.transitionIndex != transitionIndex {
					//someone else started a transition while we were sleeping--cancel the rest of this transition
					return
				}
			}
		}
		//transition done, sleep until next cycle
		mfv.transitioning = false
		mfv.currentFrame = frameIndex
		time.Sleep(mfv.frames[frameIndex].duration)
		if mfv.transitionIndex == transitionIndex {
			nextFrameIndex := (frameIndex+1)%len(mfv.frames)
			mfv.beginTransition(nextFrameIndex, mfv.frames[nextFrameIndex].transition)
		}
	}()
	mfv.draw()
}

func (mfv *MultiFrameView) draw() {
	newFrame := MakeColorFrame(mfv.display.Width, mfv.display.Height, MakeColor(0,0,0))

	frame := mfv.frames[mfv.currentFrame]
	newFrame.SetRect(frame.x, frame.y, *frame.frame, Clip)
	if mfv.transitioning {
		//draw next frame
		nextFrame := mfv.frames[(mfv.currentFrame + 1) % len(mfv.frames)]
		newFrame.SetRect(nextFrame.x, nextFrame.y, *nextFrame.frame, Clip)
	}

	mfv.display.SetFrame(newFrame)
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
