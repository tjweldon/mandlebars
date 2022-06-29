package main

import (
	"encoding/json"
	"fmt"
	"github.com/alexflint/go-arg"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"math/cmplx"
	"os"
	"reflect"
)

type Side int

const (
	Top Side = iota
	Right
	Bottom
	Left
	Sample
)

type SideOffsets [5]complex128

type View struct {
	Resolution            image.Point
	Height, Width, Aspect float64
	Centre                complex128
	Offsets               SideOffsets
}

func NewView(resolution image.Point, height float64, centre complex128) *View {
	v := &View{
		Resolution: resolution,
		Height:     height,
		Centre:     centre,
	}
	v.Aspect = v.aspect()
	v.Width = v.width()
	v.Offsets = SideOffsets{
		Top:    complex(0.0, v.Height/2.0),
		Right:  complex(v.Width/2.0, 0.0),
		Bottom: complex(0.0, -v.Height/2.0),
		Left:   complex(-v.Width/2.0, 0.0),
		Sample: complex(v.Width/float64(2*v.Resolution.X), v.Height/float64(2*v.Resolution.Y)),
	}

	return v
}

func (v View) SampleCount() int {
	return v.Resolution.X * v.Resolution.Y
}

func (v *View) aspect() float64 {
	return float64(v.Resolution.X) / float64(v.Resolution.Y)
}

func (v *View) width() float64 {
	return v.Aspect * v.Height
}

func (v *View) Samples(rowStart, rowStop int) <-chan complex128 {
	totalOffset := v.Offsets[Top] +
		v.Offsets[Left] +
		v.Offsets[Sample] +
		v.Centre
	genSamples := func(out chan<- complex128) {
		sep := complex(
			v.Width/float64(v.Resolution.X),
			-v.Height/float64(v.Resolution.Y),
		)
		for y := rowStart; y < min(v.Resolution.Y, rowStop); y++ {
			for x := 0; x < v.Resolution.X; x++ {
				out <- complex(
					real(sep)*float64(x),
					imag(sep)*float64(y),
				) + totalOffset
			}
		}
		close(out)
	}

	samples := make(chan complex128)
	go genSamples(samples)

	return samples
}

func (v *View) Points(rowStart int, rowStop int) chan image.Point {
	genPixels := func(out chan<- image.Point) {
		for y := rowStart; y < min(v.Resolution.Y, rowStop); y++ {
			for x := 0; x < v.Resolution.X; x++ {
				out <- image.Point{X: x, Y: y}
			}
		}

		close(out)
	}
	pixels := make(chan image.Point)
	go genPixels(pixels)
	return pixels
}

func (v *View) SamplePoints(rowStart, rowStop int) (<-chan complex128, <-chan image.Point) {
	samples := v.Samples(rowStart, rowStop)
	pixels := v.Points(rowStart, rowStop)

	return samples, pixels
}

func (v *View) Index(p image.Point) int {
	return p.X + v.Resolution.X*p.Y
}

const OneThird = 2.0 * math.Pi / 3

type PaletteConf struct {
	PhaseIncrement float64
	ColorFreq      float64
	HueOffset      float64
	AlphaDecay     float64
}

func (p PaletteConf) palette(n int) color.Color {
	if n == -1 {
		return color.Black
	}

	phaseIncrement := p.PhaseIncrement
	angularSpeed := p.ColorFreq * 2.0 * math.Pi / 18.0
	baseOffset := 0.0 + p.HueOffset*2.0*math.Pi
	phases := [3]float64{
		baseOffset,
		baseOffset + phaseIncrement,
		baseOffset + 2*phaseIncrement,
	}
	t := angularSpeed * float64(n)
	return color.RGBA{
		R: byte(40 + 215*math.Pow(math.Cos(t+phases[0]), 2.0)),
		G: byte(40 + 215*math.Pow(math.Cos(t+phases[1]), 2.0)),
		B: byte(40 + 215*math.Pow(math.Cos(t+phases[2]), 2.0)),
		A: byte(255.0 * math.Pow(p.AlphaDecay, float64(n))),
	}
}

// MakePalette takes the values configured in PaletteConf and returns
// a Palette function that closes around them
func (p PaletteConf) MakePalette() func(n int) color.Color {
	return p.palette
}

// Palette is the global colour Palette function
var Palette = func(n int) color.Color {
	return color.Black
}

// WorkerCount is the number of separate goroutines
// that are each given a roughly equal chunk of the image
// to render
type WorkerCount int

const (
	One WorkerCount = 1 << iota
	Two
	Four
	Eight

	// Sixteen is probably too aggressive if you care about your machine
	Sixteen
	ThirtyTwo
)

// workerCount is the number of rows to split the image into
// feel free to change this appropriately for your system.
// Set to Eight by default
const workerCount = Eight

const workerFail = -1

// RunWorker launches the goroutine that checks pixel by pixel, how many iterations it takes
// before the series diverges. If it reaches args.MaxIter then it represents this as -1
// which is rendered black by default.
func RunWorker(vals <-chan complex128, points <-chan image.Point, max int) <-chan [3]int {
	work := func(result chan<- [3]int) {
		for point := range points {
			sample, ok := <-vals
			if !ok {
				break
			}
			optN := DivergesWithin(sample, max, args.Exponent)
			if optN != nil {
				result <- [3]int{point.X, point.Y, *optN}
			} else {
				result <- [3]int{point.X, point.Y, workerFail}
			}
		}
		close(result)
	}

	ch := make(chan [3]int, 128)
	go work(ch)
	return ch
}

// SetPixels collects the sample escape times and pixel locations from their respective generators and sets them in the image object
func (v *View) SetPixels(resultChans [workerCount]<-chan [3]int, img *image.RGBA) {
	// closed stores whether each worker has closed their channel
	var closed [workerCount]bool
	closedCount, pixCount := 0, 0
	for closedCount < int(workerCount) {
		closedCount = 0
		for i, rc := range resultChans {
			// count the workers whose return channels are open
			if closed[i] {
				closedCount++
				if closedCount == int(workerCount) {
					break
				}
			}

			// iterate over channels, non-blocking.
			select {
			case pix, open := <-rc:
				if !open {
					closed[i] = true
				} else {
					img.Set(pix[0], pix[1], Palette(pix[2]))
					pixCount++
				}
			default:
			}

			// print percent completion once per row's worth of pixels
			if (pixCount+1)%v.Resolution.X == 0 {
				fmt.Printf("%05.2f%%\r", float64(100*pixCount)/float64(v.SampleCount()))
			}
		}
	}

	fmt.Println()
	fmt.Println("Done generating")
}

func StartWork(v *View, max int) [workerCount]<-chan [3]int {
	resultChans := [workerCount]<-chan [3]int{}
	for workers := 1; workers <= int(workerCount); workers++ {
		step := v.Resolution.Y / int(workerCount)
		start, stop := step*(workers-1), step*workers
		vals, points := v.SamplePoints(start, stop)
		resultChans[workers-1] = RunWorker(vals, points, max)
	}
	return resultChans
}

// DivergesWithin is the function
func DivergesWithin(c complex128, max int, exponent float64) *int {
	if args.Exponent == 2.0 {
		r := cmplx.Abs(c - 0.25)
		if r == 0 {
			return nil
		}
		theta := math.Acos(real(c-0.25) / r)
		if r < 0.5*(1-math.Cos(theta)) {
			return nil
		}
	}
	var z complex128
	for n := 0; n < max; n++ {
		if exponent == 2.0 {
			z = z*z + c
		} else {
			z = cmplx.Pow(z, complex(exponent, 0)) + c
		}
		if cmplx.Abs(z) >= 2 {
			return &n
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type ioSubcommand struct {
	Path string `arg:"positional" help:"The path to the arg spec json file"`
}

// Load is the subCommand that loads an arg spec
type Load ioSubcommand

func (l *Load) Run() error {

	file, err := os.Open(l.Path)
	if err != nil {
		return err
	}
	argBytes, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(argBytes, &args)
	if err != nil {
		return err
	}

	return nil
}

// Dump is the ioSubcommand that dumps the arg spec as json
type Dump ioSubcommand

func (d *Dump) Run() error {
	args.Dump, args.Load = nil, nil

	file, err := os.OpenFile(d.Path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Println(reflect.TypeOf(args))
	specBytes, err := json.MarshalIndent(args, "", "  ")
	var n int
	n, err = file.Write(specBytes)
	fmt.Println(n, " bytes")
	if err != nil {
		return err
	}
	return nil
}

type Cli struct {
	MaxIter     int     `arg:"--iter" default:"64" help:"The number of iterations to apply z -> z^2 + c. The actual number of iterations for a pixel is at most this value, less if it doesn't come out black."`
	PixelWidth  int     `arg:"--pixel-width" default:"1920" help:"The number of pixels per row"`
	PixelHeight int     `arg:"--pixel-height" default:"1080" help:"The number of rows of pixels"`
	Exponent    float64 `arg:"--exp" default:"2" help:"The mandlebrot set has exponent 2 (i.e. x -> z^2 + c) but we can try others!"`
	CenterReal  float64 `arg:"-r, --center-real" default:"-1.0" help:"The real (x axis) part of the complex number corresponding to the centre of the image"`
	CenterImag  float64 `arg:"-i, --center-imag" default:"0.0" help:"The imaginary (y axis) part of the complex number corresponding to the centre of the image"`
	Height      float64 `arg:"-h, --height" default:"2.0" help:"The height of the imaged region of the complex plane (not the resolution)."`
	ColorFreq   float64 `arg:"-f, --freq" default:"1.0" help:"How fast the hue varies, a smaller value means more uniform colour, more iterations means more variation close to the boundary."`
	HueOffset   float64 `arg:"--hue" default:"0.0" help:"The absolute hue offset. This is periodic such that --hue=1 and --hue=0 are the same."`
	AlphaDecay  float64 `arg:"--alpha-decay" default:"1.0" help:"A value between 0 and 1, where 0.5 means that the nth colour has (0.5)^n times 100% alpha. i.e. the colours fade close to the boundary. A value of 1 is no decay."`
	Load        *Load   `arg:"subcommand:load" help:"Load image spec json from path" json:"-"`
	Dump        *Dump   `arg:"subcommand:dump" help:"Dump options to arg spec json file. Dumps defaults if no options are set" json:"-"`
}

func (c Cli) Palette() func(n int) color.Color {
	return PaletteConf{
		PhaseIncrement: OneThird,
		ColorFreq:      c.ColorFreq,
		HueOffset:      c.HueOffset,
		AlphaDecay:     c.AlphaDecay,
	}.MakePalette()
}

var args Cli

func main() {
	arg.MustParse(&args)
	Palette = args.Palette()
	var err error
	switch {
	case args.Dump != nil:
		log.Println("Dumping...")
		err = args.Dump.Run()
		log.Println("Done")
		return
	case args.Load != nil:
		log.Println("Loading")
		err = args.Load.Run()
	}

	if err != nil {
		log.Fatal(err)
	}

	v := NewView(
		image.Point{
			X: args.PixelWidth,
			Y: args.PixelHeight,
		},
		args.Height,
		complex(args.CenterReal, args.CenterImag),
	)
	img := image.NewRGBA(image.Rect(0, 0, v.Resolution.X, v.Resolution.Y))
	for x := 0; x < v.Resolution.X; x++ {
		for y := 0; y < v.Resolution.Y; y++ {
			img.Set(x, y, color.Black)
		}
	}

	max := args.MaxIter
	resultChans := StartWork(v, max)

	v.SetPixels(resultChans, img)

	f, _ := os.Create("image.png")
	err = png.Encode(f, img)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Done")
}
