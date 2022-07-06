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
	"mandlebars/src/palette"
	"mandlebars/src/view"
	"math"
	"math/cmplx"
	"os"
	"os/exec"
)

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

func StartWork(v *view.View, max int) [workerCount]<-chan [3]int {
	resultChans := [workerCount]<-chan [3]int{}
	for workers := 1; workers <= int(workerCount); workers++ {
		step := v.Resolution.Y / int(workerCount)
		start, stop := step*(workers-1), step*workers
		vals, points := v.SamplePoints(start, stop)
		resultChans[workers-1] = RunWorker(vals, points, max)
	}
	return resultChans
}

// SetPixels collects the sample escape times and pixel locations from their respective generators and sets them in the image object
func SetPixels(resultChans [workerCount]<-chan [3]int, img *image.RGBA, v *view.View) {
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

			// print percent completion once per row's worth of pixels if not in stdout mode
			if !args.StdOut && (pixCount+1)%v.Resolution.X == 0 {
				fmt.Printf("%05.2f%%\r", float64(100*pixCount)/float64(v.SampleCount()))
			}
		}
	}

	if !args.StdOut {
		fmt.Println()
		fmt.Println("Done generating")
	}
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

type ioSubcommand struct {
	Path string `arg:"positional" help:"The path to the file"`
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

	specBytes, err := json.MarshalIndent(args, "", "  ")
	var n int
	n, err = file.Write(specBytes)
	fmt.Println(n, " bytes")
	if err != nil {
		return err
	}
	return nil
}

// OutFile lets you put the image file where you want it
type OutFile ioSubcommand

func (o *OutFile) Run() error {
	dst = o.Path
	return nil
}

type Cli struct {
	MaxIter     int      `arg:"--iter" default:"64" help:"The number of iterations to apply z -> z^2 + c. The actual number of iterations for a pixel is at most this value, less if it doesn't come out black."`
	PixelWidth  int      `arg:"--pixel-width" default:"1920" help:"The number of pixels per row"`
	PixelHeight int      `arg:"--pixel-height" default:"1080" help:"The number of rows of pixels"`
	Display     int      `arg:"--display-height" default:"-1" help:"Uses goiv to display an image" json:"-"`
	Exponent    float64  `arg:"--exp" default:"2" help:"The mandlebrot set has exponent 2 (i.e. x -> z^2 + c) but we can try others!"`
	CenterReal  float64  `arg:"-r, --center-real" default:"-1.0" help:"The real (x axis) part of the complex number corresponding to the centre of the image"`
	CenterImag  float64  `arg:"-i, --center-imag" default:"0.0" help:"The imaginary (y axis) part of the complex number corresponding to the centre of the image"`
	Height      float64  `arg:"-h, --height" default:"2.0" help:"The height of the imaged region of the complex plane (not the resolution)."`
	ColorFreq   float64  `arg:"-f, --freq" default:"1.0" help:"How fast the hue varies, a smaller value means more uniform colour, more iterations means more variation close to the boundary."`
	HueOffset   float64  `arg:"--hue" default:"0.0" help:"The absolute hue offset. This is periodic such that --hue=1 and --hue=0 are the same."`
	AlphaDecay  float64  `arg:"--alpha-decay" default:"1.0" help:"A value between 0 and 1, where 0.5 means that the nth colour has (0.5)^n times 100% alpha. i.e. the colours fade close to the boundary. A value of 1 is no decay."`
	Load        *Load    `arg:"subcommand:load" help:"Load image spec json from path" json:"-"`
	Dump        *Dump    `arg:"subcommand:dump" help:"Dump options to arg spec json file. Dumps defaults if no options are set" json:"-"`
	OutFile     *OutFile `arg:"subcommand:to" help:"Saves the image to the specified path" json:"-"`
	StdOut      bool     `arg:"--stdout" help:"The image data will be output to stdout" json:"-"`
}

func (c Cli) Palette() func(n int) color.Color {

	return palette.PaletteConf{
		PhaseIncrement: palette.OneThird,
		ColorFreq:      c.ColorFreq,
		HueOffset:      c.HueOffset,
		AlphaDecay:     c.AlphaDecay,
	}.MakePalette()
}

var args Cli

var dst = "./mandle.png"

// GenerateImage sets up the view.View, spawns the workers and then supplies the result
// channels to SetPixels. The image is then written the supplied path
func GenerateImage(path string) *view.View {
	v := view.NewView(
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

	SetPixels(resultChans, img, v)

	var (
		f   io.WriteCloser
		err error
	)

	if args.StdOut {
		f = os.Stdout
	} else {
		f, err = os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = png.Encode(f, img)
	if err != nil {
		log.Fatal(err)
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}

	return v
}

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
	if args.OutFile != nil {
		err = args.OutFile.Run()
	}

	if err != nil {
		log.Fatal(err)
	}

	view := GenerateImage(dst)

	if args.Display >= 0 {
		subprocessArgs := []string{dst}
		if args.Display > 0 {
			w, h := int(float64(args.Display)*view.Aspect), args.Display
			subprocessArgs = []string{"-w", fmt.Sprint(w), "-h", fmt.Sprint(h), dst}
		}

		cmd := exec.Command("goiv", subprocessArgs...)
		err := cmd.Run()
		if err != nil {
			return
		}
	}
	log.Println("Done")
}
