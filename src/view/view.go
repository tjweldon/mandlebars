package view

import (
	"image"
	"mandlebars/src/util"
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
		for y := rowStart; y < util.Min(v.Resolution.Y, rowStop); y++ {
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
		for y := rowStart; y < util.Min(v.Resolution.Y, rowStop); y++ {
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
