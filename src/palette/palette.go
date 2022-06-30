package palette

import (
	"image/color"
	"math"
)

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
