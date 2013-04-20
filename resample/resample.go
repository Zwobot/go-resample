package resample

import (
	"image"
	"image/color"
	"math"
)

const epsilon = 0.0000125
const lanczosWindow = 3.0

func sinc(f float64) float64 {
	f *= math.Pi
	if f < 0.01 && f > -0.01 {
		// Taylor expansion
		return 1.0 + f*f*(-1.0/6.0+f*f*1.0/120.0)
	}
	return math.Sin(f) / f
}

func cutnoise(f float64) float64 {
	// Return 0 if f is NaN. (Any NaN comparison is false).
	if f < -epsilon || f > epsilon {
		return f
	}
	return 0.0
}

func lanczos(w float64) func(f float64) float64 {
	return func(f float64) float64 {
		if f < 0 {
			f = -f
		}
		if f < w {
			return cutnoise(sinc(f) * sinc(f/w))
		}
		return 0.0
	}
}

type Filter struct {
	Apply   func(float64) float64
	Support float64
}

func box(x float64) float64 {
	if -0.5 < x && x <= 0.5 {
		return 1.0
	}
	return 0.0
}

func triangle(x float64) float64 {
	if x > 0 {
		x = -x
	}
	if x > -1 {
		return x
	}
	return 0
}

var (
	Lanczos12 = Filter{Apply: lanczos(12), Support: 12}
	Lanczos5  = Filter{Apply: lanczos(5), Support: 5}
	Lanczos3  = Filter{Apply: lanczos(3), Support: 3}
	Box       = Filter{Apply: box, Support: 0.5}
	Triangle  = Filter{Apply: triangle, Support: 1}
)

type WrapFunc func(x, min, max int) int

// Some example boundary wrapper functions
var (
	Clamp = func(x, min, max int) int {
		switch {
		case x < min:
			x = min
		case x > max:
			x = max
		}
		return x
	}

	Reject = func(x, min, max int) int {
		// The code already rejects out of bounds return values.
		// So we don't need to do anything
		return x
	}
)

func ResampleNRGBA64(dst, src *image.NRGBA64, f Filter, xWrap, yWrap WrapFunc) error {
	// fmt.Printf("===> sampling filter in x ...\n")
	x_filter := makeDiscreteFilter(f, xWrap, dst.Bounds().Dx(), src.Bounds().Dx())

	// fmt.Printf("===> sampling filter in y ...\n")
	y_filter := makeDiscreteFilter(f, yWrap, dst.Bounds().Dy(), src.Bounds().Dy())

	// fmt.Printf("===> folding Y ...\n")
	tmp := image.NewNRGBA64(image.Rect(0, 0, src.Bounds().Dx(), dst.Bounds().Dy()))
	resampleAxisNRGBA64(YAxis, tmp, src, y_filter)

	// fmt.Printf("===> folding X ...\n")
	resampleAxisNRGBA64(XAxis, dst, tmp, x_filter)
	return nil
}

type fRGBA64 struct {
	R, G, B, A float64
}

func clampF64ToUint16(x float64) uint16 {
	if x > float64(uint16(0xffff)) {
		return uint16(0xffff)
	}
	if x < 0 {
		return 0
	}
	return uint16(x) // What happens with NaNs?
}

type axisSwitch int

const (
	YAxis axisSwitch = iota
	XAxis
)

type kvPair struct {
	k int
	v float64
}

func makeDiscreteFilter(f Filter, wrap WrapFunc, ndst, nsrc int) [][]kvPair {
	df := make([][]kvPair, ndst)
	dst2src := float64(ndst) / float64(nsrc)
	support := f.Support
	fscale := 1.0
	if dst2src < 1.0 {
		// Downsampling.
		support /= dst2src
		fscale *= dst2src
	}
	nudge := 1e-8 
	for i := 0; i != ndst; i++ {
		src_x := float64(i) / dst2src
		min := int(math.Floor(src_x - support - nudge))
		max := int(math.Ceil(src_x + support + nudge))
		df[i] = make([]kvPair, 0, max-min+1)
		for j := min; j <= max; j++ {
			v := f.Apply(fscale*(float64(j)-src_x)) * fscale
			k := wrap(j, 0, nsrc-1)
			if 0 <= k && k < nsrc && v != 0 {
				df[i] = append(df[i], kvPair{k, v})
			}
		}
	}
	return df
}

const uint16tof64 = 1.0 / float64(uint16(0xffff))

func fillLineFRGBA64(flipXY bool, column []fRGBA64, x int, src *image.NRGBA64) {
	dy := src.Bounds().Min.Y
	dx := src.Bounds().Min.X
	pix := src.Pix
	var idx int
	for y := 0; y != len(column); y++ {
		if flipXY {
			idx = src.PixOffset(y+dx, x+dy)
		} else {
			idx = src.PixOffset(x+dx, y+dy)
		}
		column[y].R = uint16tof64 * float64(uint16(pix[idx+0])<<8|uint16(pix[idx+1]))
		column[y].G = uint16tof64 * float64(uint16(pix[idx+2])<<8|uint16(pix[idx+3]))
		column[y].B = uint16tof64 * float64(uint16(pix[idx+4])<<8|uint16(pix[idx+5]))
		column[y].A = uint16tof64 * float64(uint16(pix[idx+6])<<8|uint16(pix[idx+7]))
	}
}

// Resample axis..
func resampleAxisNRGBA64(axis axisSwitch, dst, src *image.NRGBA64, f [][]kvPair) {
	flip := axis != YAxis

	dst_bbox := dst.Bounds()
	src_bbox := src.Bounds()

	dst_min_x, dst_max_x := dst_bbox.Min.X, dst_bbox.Max.X
	dst_min_y, dst_max_y := dst_bbox.Min.Y, dst_bbox.Max.Y
	ysize := src_bbox.Dy()
	xsize := src_bbox.Dx()

	if flip {
		xsize, ysize = ysize, xsize
		dst_min_x, dst_min_y = dst_min_y, dst_min_x
		dst_max_x, dst_max_y = dst_max_y, dst_max_x
	}

	if dst_max_x-dst_min_x != xsize {
		panic("Axis must be preserved.")
	}

	src_column := make([]fRGBA64, ysize)

	for x := dst_min_x; x != dst_max_x; x++ {
		fillLineFRGBA64(flip, src_column, x, src)
		// Resample column
		y_i := 0
		for y := dst_min_y; y != dst_max_y; y++ {
			var dst_c fRGBA64
			for _, f_y := range f[y_i] {
				dst_c.R += f_y.v * src_column[f_y.k].R
				dst_c.G += f_y.v * src_column[f_y.k].G
				dst_c.B += f_y.v * src_column[f_y.k].B
				dst_c.A += f_y.v * src_column[f_y.k].A
			}
			dst_nrgba := color.NRGBA64{
				R: clampF64ToUint16((256*256 - 1) * dst_c.R),
				G: clampF64ToUint16((256*256 - 1) * dst_c.G),
				B: clampF64ToUint16((256*256 - 1) * dst_c.B),
				A: clampF64ToUint16((256*256 - 1) * dst_c.A),
			}
			if flip {
				dst.SetNRGBA64(y, x, dst_nrgba)
			} else {
				dst.SetNRGBA64(x, y, dst_nrgba)
			}
			y_i++
		}
	}
}
