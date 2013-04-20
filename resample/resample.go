package resample

import (
	"image"
	"image/color"
	"math"
)

const epsilon = 0.0000125

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
		return 1+x
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

	// Count samples outside the boundaries as black.
	Reject = func(x, min, max int) int {
		// The code already rejects out of bounds return values.
		// So we don't need to do anything
		return x
	}

	Reflect = func(x, min, max int) int {
		switch {
		case x < min:
			return 2*min - x
		case x > max:
			return 2*max - x
		}
		return x
	}
)

// Mid-level image scaling function.
//
// The source image src is sampled with the supplied
// filter and boundary handling functions.
func Resample(dst *image.NRGBA64, src image.Image, f Filter, xWrap, yWrap WrapFunc) error {
	x_filter := makeDiscreteFilter(f, xWrap, dst.Bounds().Dx(), src.Bounds().Dx())
	y_filter := makeDiscreteFilter(f, yWrap, dst.Bounds().Dy(), src.Bounds().Dy())

	tmp := image.NewNRGBA64(image.Rect(0, 0, src.Bounds().Dx(), dst.Bounds().Dy()))
	resampleAxisNRGBA64(YAxis, tmp, src, y_filter)
	resampleAxisNRGBA64(XAxis, dst, tmp, x_filter)
	return nil
}

type f32RGBA struct {
	R, G, B, A float32
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

func clampF32ToUint16(x float32) uint16 {
	if x > float32(uint16(0xffff)) {
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
	v float32
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
				df[i] = append(df[i], kvPair{k, float32(v)})
			}
		}
	}
	return df
}

const (
	uint16_to_f32 = 1.0 / float32(uint16(0xffff))
	f32_to_uint16 = float32(uint16(0xffff))
)

func fetchLineNRGBA64(flipXY bool, column []f32RGBA, x int, src *image.NRGBA64) {
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
		column[y].R = uint16_to_f32 * float32(uint16(pix[idx+0])<<8|uint16(pix[idx+1]))
		column[y].G = uint16_to_f32 * float32(uint16(pix[idx+2])<<8|uint16(pix[idx+3]))
		column[y].B = uint16_to_f32 * float32(uint16(pix[idx+4])<<8|uint16(pix[idx+5]))
		column[y].A = uint16_to_f32 * float32(uint16(pix[idx+6])<<8|uint16(pix[idx+7]))
	}
}

func fetchLine(flipXY bool, column []f32RGBA, x int, src image.Image) {
	switch src := src.(type) {
	case *image.NRGBA64:
		fetchLineNRGBA64(flipXY,column,x,src)
		return

	}
	dy := src.Bounds().Min.Y
	dx := src.Bounds().Min.X
	var r,g,b,a uint32
	for y := 0; y != len(column); y++ {
		if flipXY {
			r,g,b,a = src.At(y+dx,x+dy).RGBA()
		} else {
			r,g,b,a = src.At(x+dx,y+dy).RGBA()
		}
		column[y].R = uint16_to_f32 * float32(r)
		column[y].G = uint16_to_f32 * float32(g)
		column[y].B = uint16_to_f32 * float32(b)
		column[y].A = uint16_to_f32 * float32(a)
	}
}

func putLineNRGBA64(flipXY bool, column []f32RGBA, x int, dst *image.NRGBA64) {
	dy := dst.Bounds().Min.Y
	dx := dst.Bounds().Min.X
	for y, dst_c := range column {
		dst_nrgba := color.NRGBA64{
				R: clampF32ToUint16(f32_to_uint16 * dst_c.R),
				G: clampF32ToUint16(f32_to_uint16 * dst_c.G),
				B: clampF32ToUint16(f32_to_uint16 * dst_c.B),
				A: clampF32ToUint16(f32_to_uint16 * dst_c.A),
		}
		if flipXY {
			dst.SetNRGBA64(y+dy, x+dx, dst_nrgba)
		} else {
			dst.SetNRGBA64(x+dx, y+dy, dst_nrgba)
		}
	}
}

// Resample axis..
func resampleAxisNRGBA64(axis axisSwitch, dst *image.NRGBA64, src image.Image, f [][]kvPair) {
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

	src_column := make([]f32RGBA, ysize)
	dst_column := make([]f32RGBA, dst_max_y - dst_min_y)

	for x := dst_min_x; x != dst_max_x; x++ {
		fetchLine(flip, src_column, x, src)
		y_i := 0
		for y := dst_min_y; y != dst_max_y; y++ {
			var dst_c f32RGBA
			for _, f_y := range f[y_i] {
				src_c := src_column[f_y.k]
				dst_c.R += f_y.v * src_c.R
				dst_c.G += f_y.v * src_c.G
				dst_c.B += f_y.v * src_c.B
				dst_c.A += f_y.v * src_c.A
			}
			dst_column[y_i] = dst_c
			y_i++
		}
		putLineNRGBA64(flip, dst_column, x, dst)
	}
}
