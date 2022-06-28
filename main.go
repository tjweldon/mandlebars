package main

import (
	"fmt"
	"github.com/alexflint/go-arg"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/cmplx"
	"os"
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

func (v *View) SampleVals(rowStart, rowStop int) (<-chan complex128, <-chan image.Point) {
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
				) +
					v.Offsets[Top] +
					v.Offsets[Left] +
					v.Offsets[Sample] +
					v.Centre
			}
		}
		close(out)
	}

	genPixels := func(out chan<- image.Point) {
		for y := rowStart; y < min(v.Resolution.Y, rowStop); y++ {
			for x := 0; x < v.Resolution.X; x++ {
				out <- image.Point{X: x, Y: y}
			}
		}

		close(out)
	}

	samples := make(chan complex128)
	pixels := make(chan image.Point)
	go genSamples(samples)
	go genPixels(pixels)

	return samples, pixels
}

func (v *View) Index(p image.Point) int {
	return p.X + v.Resolution.X*p.Y
}

func Pallette(n int) color.Color {
	if n == -1 {
		return color.Black
	}

	phaseIncrement := 2.0 * math.Pi / 3
    angularSpeed := 2.0 * math.Pi / (args.ColorFreq*18.0)
	baseOffset := 0.0
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
		A: 255,
	}
}

type WorkerCount int

const (
	One WorkerCount = 1 << iota
	Two
	Four
	Eight
	Sixteen
	ThirtyTwo
)

const workerCount = Sixteen

func worker(vals <-chan complex128, points <-chan image.Point, v *View, start, stop, max int) <-chan [3]int {
	work := func(result chan<- [3]int) {
		for point := range points {
			sample, ok := <-vals
			if !ok {
				break
			}
			optN := DivergesWithin(sample, max)
			if optN != nil {
				result <- [3]int{point.X, point.Y, *optN}
			} else {
				result <- [3]int{point.X, point.Y, -1}
			}
		}
		close(result)
	}

	ch := make(chan [3]int, 128)
	go work(ch)
	return ch
}

var args struct {
	MaxIter int `arg:"positional"`
    CenterReal float64 `arg:"-r, --center-real" default:"-1.0"`
    CenterImag float64 `arg:"-i, --center-imag" default:"0.0"`
    Height float64 `arg:"-h, --height" default:"2.0"`
    ColorFreq float64 `arg:"-f, --freq" default:"2.0"`
}

func main() {
	arg.MustParse(&args)
	v := NewView(
		image.Point{
			X: 3440 * 2,
			Y: 1440 * 2,
		},
		args.Height,
		complex(args.CenterReal, args.CenterImag),
    )
	img := image.NewRGBA(image.Rect(0, 0, v.Resolution.X, v.Resolution.Y))

	max := args.MaxIter
	resultChans := spawnWorkerPool(v, max)

	setPixels(resultChans, img, v)

	f, _ := os.Create("image.png")
	png.Encode(f, img)
}

func setPixels(resultChans [16]<-chan [3]int, img *image.RGBA, v *View) {
	var closed [workerCount]bool
	closedCount, pixCount := 0, 0
	for closedCount < int(workerCount) {
		closedCount = 0
		for i, rc := range resultChans {
			if closed[i] {
				closedCount++
				if closedCount == int(workerCount) {
					break
				}
			}
			select {
			case pix, open := <-rc:
				if !open {
					closed[i] = true
				} else {
					img.Set(pix[0], pix[1], Pallette(pix[2]))
					pixCount++
				}
			default:
			}
			if (pixCount+1)%v.Resolution.X == 0 {
				fmt.Printf("%05.2f%%\r", float64(100*pixCount)/float64(v.SampleCount()))
			}
		}
	}

	fmt.Println()
	fmt.Println("Done generating")
}

func spawnWorkerPool(v *View, max int) [16]<-chan [3]int {
	resultChans := [workerCount]<-chan [3]int{}
	for workers := 1; workers <= int(workerCount); workers++ {
		step := v.Resolution.Y / int(workerCount)
		start, stop := step*(workers-1), step*workers
		vals, points := v.SampleVals(start, stop)
		resultChans[workers-1] = worker(vals, points, v, start, stop, max)
	}
	return resultChans
}

func DivergesWithin(c complex128, max int) *int {
	r := cmplx.Abs(c - 0.25)
	if r == 0 {
		return nil
	}
	theta := math.Acos(real(c-0.25) / r)
	if r < 0.5*(1-math.Cos(theta)) {
		return nil
	}

	var z complex128
	for n := 0; n < max; n++ {
		z = z*z + c
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
