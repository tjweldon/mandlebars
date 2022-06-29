# Mandlebars

A fractal generator that doubles as a total conversion mod to turn your computer into a fan heater.

## Usage

```bash
Usage: main [--exp EXP] [--pixel-width PIXEL-WIDTH] [--pixel-height PIXEL-HEIGHT] [--iter ITER] [--center-real CENTER-REAL] [--center-imag CENTER-IMAG] [--height HEIGHT] [--freq FREQ] [--hue HUE] [--alpha-decay ALPHA-DECAY]

Options:
  --exp EXP              The mandlebrot set has exponent 2 (i.e. x -> z^2 + c) but we can try others! [default: 2]
  --pixel-width PIXEL-WIDTH
                         The number of pixels per row [default: 1920]
  --pixel-height PIXEL-HEIGHT
                         The number of rows of pixels [default: 1080]
  --iter ITER            The number of iterations to apply z -> z^2 + c. The actual number of iterations for a pixel is at most this value, less if it doesn't come out black. [default: 64]
  --center-real CENTER-REAL, -r CENTER-REAL
                         The real (x axis) part of the complex number corresponding to the centre of the image [default: -1.0]
  --center-imag CENTER-IMAG, -i CENTER-IMAG
                         The imaginary (y axis) part of the complex number corresponding to the centre of the image [default: 0.0]
  --height HEIGHT, -h HEIGHT
                         The height of the imaged region of the complex plane (not the resolution). [default: 2.0]
  --freq FREQ, -f FREQ   How fast the hue varies, a smaller value means more uniform colour, more iterations means more variation close to the boundary. [default: 1.0]
  --hue HUE              The absolute hue offset. This is periodic such that --hue=1 and --hue=0 are the same. [default: 0.0]
  --alpha-decay ALPHA-DECAY
                         A value between 0 and 1, where 0.5 means that the nth colour has (0.5)^n times 100% alpha. i.e. the colours fade close to the boundary. A value of 1 is no decay. [default: 1.0]
  --help, -h             display this help and exit
```

## Example


```bash
go run main.go --iter=40000 --height=0.004 -r 0.28 -i -0.01 --pixel-width=6880 --pixel-height=2880 --freq=0.02 --hue=0.4
```
**Warning**: This takes a long time and a LOT of compute, but it's pretty though.


![example.png](doc/iter-40k-centre-0.28r0.01i-height-0.004-freq-0.02-hue0.4.png)
