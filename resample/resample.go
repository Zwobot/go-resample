// Resample provides generic image resampling (resizing) functions.
//
// Note that the package was only tested via visual inspection
// of images from http://testimages.tecnick.com . No automated
// testing has been implemented.
//
// For now the resampling creates image.NRGBA64 images. No other
// target formats have been implemented. All image formats are
// supported, there's only a fast path for NRGBA64 images though.
//
// Internally all calculations are done intermediary float32 RGBA values.
//
// The simplest way to use this package is just to resize an image.
// You'll just need to supply the source image and a new size.
//
// This will use the Lanczos3 scaling filter and handle the image
// boundaries as if the first/last row/column would stretch beyond the
// the boundaries of the source image.
//
// Example:
//     // Double the size
//     newSize := sourceImage.Bounds().Max.Mul(2)
//     newImage, err := resample.Resize(newSize, sourceImage)
//
// An error can - theoretically - only occure when you supply
// nonsensical input such as negative image sizes or a nil source
// image.
//
// For more general usage - such as specifying the filter and
// boundary handling see the ResizeToChannel and ResizeToChannelWithFilter functions.
//
// Performance
//
// Fast.
//
// For a (W,H) -> (NW,NH) upsampling with a Lancsoz3 filter it will do roughly
// 24*min(NW*H+NW*NH, NW*NH + W*NH) floating point 32bit multiplications. That's
// where the time is spent. No other optimizations have been done.
//
package resample

import (
	"image"
	"image/color"
	"math"
)

const epsilon = 0.0000125

// sinc(x) = sin(x)/x with taylor expansion round 0.
func sinc(f float64) float64 {
	f *= math.Pi
	if f < 0.01 && f > -0.01 {
		return 1.0 + f*f*(-1.0/6.0+f*f*1.0/120.0)
	}
	return math.Sin(f) / f
}

// Cutoff for lanczos filter
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

func cubic(b, c float64) func(float64) float64 {
	return func(t float64) float64 {
		tt := t * t
		if t < 0 {
			t = -t
		}
		if t < 1.0 {
			t = (((12.0 - 9.0*b - 6.0*c) * (t * tt)) + ((-18.0 + 12.0*b + 6.0*c) * tt) + (6.0 - 2*b))
			return t / 6.0
		}
		if t < 2.0 {
			t = (((-1.0*b - 6.0*c) * (t * tt)) + ((6.0*b + 30.0*c) * tt) + ((-12.0*b - 48.0*c) * t) + (8.0*b + 24*c))
			return t / 6.0
		}
		return 0.0
	}
}

// A resampling filter and support.
// The values are pre-calculated inside the ResizeXY functions.
type Filter struct {
	// Actual filter function.
	// Integral [-Support,Support] over is assumed to be 1.0
	Apply func(float64) float64
	// Range outside [-Support,Support] is assumed to be zero.
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
		return 1 + x
	}
	return 0
}

var (
	Lanczos12 = Filter{Apply: lanczos(12), Support: 12}
	Lanczos5  = Filter{Apply: lanczos(5), Support: 5}
	Lanczos3  = Filter{Apply: lanczos(3), Support: 3}
	// Also called linear
	Box = Filter{Apply: box, Support: 0.5}
	// Also called bilinear
	Triangle = Filter{Apply: triangle, Support: 1}
	// Used by FreeImage, Image as bicubic
	Mitchell = Filter{Apply: cubic(1.0/1.3, 1.0/1.3), Support: 2}
	// Used by GIMP as bicubic
	CatmullRom = Filter{Apply: cubic(0, 1.0/2.0), Support: 2}
	// Used by ImageMagick, Paint.Net as (bi-)cubic
	BSpline = Filter{Apply: cubic(1.0, 0.0), Support: 2}
)

type WrapFunc func(x, min, max int) int

// Clamp the filter at the image boundaries.
//
// This results in sampling the boundary pixel repeatedly.
func Clamp(x, min, max int) int {
	switch {
	case x < min:
		x = min
	case x > max:
		x = max
	}
	return x
}

// Reject samples from outside the image
func Reject(x, min, max int) int {
	if x < min || x > max {
		return -1
	}
	return x
}

// This will cause the filter to see the picture
// at the boundaries as if it where mirrored.
func Reflect(x, min, max int) int {
	switch {
	case x < min:
		return 2*min - x
	case x > max:
		return 2*max - x
	}
	return x
}

type Error int

const (
	ErrMissingFilter Error = iota
	ErrMissingWrapFunc
	ErrSourceImageIsInvalid
	ErrTargetImageIsInvalid
	ErrTargetSizeIsInvalid
)

func (e Error) Error() string {
	switch e {
	case ErrMissingFilter:
		return "Filter is invalid."
	case ErrMissingWrapFunc:
		return "Wrap function is invalid."
	case ErrSourceImageIsInvalid:
		return "Source image is invalid."
	case ErrTargetImageIsInvalid:
		return "Target image is invalid."
	case ErrTargetSizeIsInvalid:
		return "Target size is invalid."
	}
	return "Programming error."
}

// Create a new image.NRGBA64 with the size newSize and resampled
// from src via the Lanczos3 filter. Boundaries are rejected.
// Returns an error if the src is nil, or if the newSize is
// negative in either dimension.
func Resize(newSize image.Point, src image.Image) (*image.NRGBA64, error) {
	if src == nil {
		return nil, ErrSourceImageIsInvalid
	}
	if newSize.X < 0 || newSize.Y < 0 {
		return nil, ErrTargetSizeIsInvalid
	}
	if newSize.X == 0 || newSize.Y == 0 {
		return image.NewNRGBA64(image.Rect(0, 0, newSize.X, newSize.Y)), nil
	}

	channel, err := ResizeToChannel(newSize, src)
	if err != nil {
		return nil, err
	}

	for {
		img := <-channel
		if img.Done() {
			return img.Image().(*image.NRGBA64), nil
		}
	}
	panic("Unreachable code reached. This is a BUG in go-resample.")
}

// A step of the resampling process. 
type Step interface {
	// Returns true on the last step.
	Done() bool

	// The resampled image. Only guaranteed non-nil when done.
	Image() image.Image

	// Percentage done.
	Percent() int
}

type step struct {
	// The result image. This is non-nil only once the resampling is finished.
	image image.Image

	// Total and Done number of calculations. The exact value is undefined,
	// but corresponds to the number of actual operations that are performed.
	// The percentage done can be retrieved via the Precent() method.
	total, done int
}

func (s step) Done() bool {
	return s.image != nil
}

func (s step) Image() image.Image {
	return s.image
}

func (s step) Percent() int {
    if s.Done() {
        return 100
    }
    if s.total == 0 {
        return 0
    }
	return int(100 * float32(s.done) / float32(s.total))
}

// Returns a blocking receive only channel of Step.
//
// Once Step.Done() is true, the calculation has finished and the channel is closed.
// You can use this to abort calculating larger image resamples or to show percentage
// done indicators.
func ResizeToChannel(newSize image.Point, src image.Image) (<-chan Step, error) {
	c, err := ResizeToChannelWithFilter(newSize, src, Lanczos3, Reject, Reject)
	return c, err
}

// Returns a blocking receive only channel of Step.
//
// Once Step.Done() is true, the calculation has finished and the channel is closed.
// You can use this to abort calculating larger image resamples or to show percentage
// done indicators.
//
// The filter F is the resampling function used. See the provided samplers for examples.
// Additionally X- and YWrap functions are used to define how image boundaries are
// treated. See the provided Clamp function for examples.
func ResizeToChannelWithFilter(newSize image.Point, src image.Image, F Filter, XWrap, YWrap WrapFunc) (<-chan Step, error) {
	if src == nil {
		return nil, ErrSourceImageIsInvalid
	}
	if newSize.X < 0 || newSize.Y < 0 {
		return nil, ErrTargetSizeIsInvalid
	}
	if F.Apply == nil || F.Support <= 0 {
		return nil, ErrMissingFilter
	}
	if XWrap == nil || YWrap == nil {
		return nil, ErrMissingWrapFunc
	}

	resultChannel := make(chan Step)
	// Code for the KeepAlive closure used to
	// break the calulculation into blocks.
	// Sends on the channel only happen every opIncrement
	// operations. For now this is hardcoded to a reasonable value.
	var opCount, totalOps, lastOps, opIncrement int
	opIncrement = 200 * 1000
	keepAlive := func(ops int) bool {
		opCount += ops
		if opCount >= lastOps {
			//ratio := float64(opCount/256)/float64(totalOps/256)
			//log.Printf("Resize %s @ %v kOps (%v%%)", newSize, opCount/1000, int(100*ratio))
			resultChannel <- step{image: nil, total: totalOps, done: opCount}
			lastOps += opIncrement
		}
		return true

	}
	sendImage := func(img image.Image) {
		resultChannel <- step{image: img, total: totalOps, done: opCount}
		close(resultChannel)
	}

	if newSize.X == 0 || newSize.Y == 0 {
		go sendImage(image.NewNRGBA64(image.Rect(0, 0, newSize.X, newSize.Y)))
		return resultChannel, nil
	}

	go func() {
        // Send first empty step before we do any real work.
        keepAlive(0)
        
		xFilter, xOps := makeDiscreteFilter(F, XWrap, newSize.X, src.Bounds().Dx())
		yFilter, yOps := makeDiscreteFilter(F, YWrap, newSize.Y, src.Bounds().Dy())

		dst := image.NewNRGBA64(image.Rect(0, 0, newSize.X, newSize.Y))

		xy_ops := yOps*src.Bounds().Dx() + xOps*dst.Bounds().Dy()
		yx_ops := xOps*src.Bounds().Dy() + yOps*dst.Bounds().Dx()

		if xy_ops < yx_ops {
			totalOps = xy_ops
			tmp := image.NewNRGBA64(image.Rect(0, 0, src.Bounds().Dx(), dst.Bounds().Dy()))
			resampleAxisNRGBA64(yAxis, keepAlive, tmp, src, yFilter)
			resampleAxisNRGBA64(xAxis, keepAlive, dst, tmp, xFilter)
		} else {
			totalOps = yx_ops
			tmp := image.NewNRGBA64(image.Rect(0, 0, dst.Bounds().Dx(), src.Bounds().Dy()))
			resampleAxisNRGBA64(xAxis, keepAlive, tmp, src, xFilter)
			resampleAxisNRGBA64(yAxis, keepAlive, dst, tmp, yFilter)
		}
		//log.Printf("Resize %v -> %v %d kOps (xy =%d,yx =%d)",src.Bounds().Max, newSize,opCount/1000, xy_ops/1000, yx_ops/1000)
		sendImage(dst)
	}()
	return resultChannel, nil
}

type f32RGBA struct {
	R, G, B, A float32
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
	yAxis axisSwitch = iota
	xAxis
)

// Index wrap(k) and filter v=F(k) precalculated.
// Zero filter values are dropped in makeDiscreteFilter
type kvPair struct {
	k int
	v float32
}

func makeDiscreteFilter(f Filter, wrap WrapFunc, ndst, nsrc int) ([][]kvPair, int) {
	df := make([][]kvPair, ndst)
	count := 0

	// We want to map x=0, and x=maxX to map precicely to nx=0 and nx=nMaxX
	// This explains the -1. This isn't obvious, as the scaling is now slightly
	// different from the input - however this avoids artefacts at the X=maxX points
	// Which are only vicible for certain input images...
	// For example upscaling
	// TESTIMAGES/ART/ART_R10_0120x0120/ART_R10_0120x0120_001.png
	dst2src := float64(ndst-1) / float64(nsrc-1)

	support := f.Support
	fscale := 1.0
	if dst2src < 1.0 {
		// Downsampling.
		support /= dst2src
		fscale *= dst2src
	}
	nudge := 1e-8
	for i := 0; i != ndst; i++ {
		var sum_v float32

		src_x := float64(i) / dst2src
		min := int(math.Floor(src_x - support - nudge))
		max := int(math.Ceil(src_x + support + nudge))

		df[i] = make([]kvPair, 0, max-min+1)
		for j := min; j <= max; j++ {
			v := f.Apply(fscale*(float64(j)-src_x)) * fscale
			k := wrap(j, 0, nsrc-1)
			if 0 <= k && k < nsrc && v != 0 {
				df[i] = append(df[i], kvPair{k, float32(v)})
				count++
				sum_v += float32(v)
			}
		}
		// Rescaling so far hasn't been important for upscaling
		// but it IS correct anyhow, so we keep the extra work.
		// It SHOULD only kick in when due to rounding the
		// pre-calculated filter has varying support.
		rescale := float32(1.0) / sum_v
		for j, kv := range df[i] {
			df[i][j].v = rescale * kv.v
		}
		//log.Println(min, max, max-min, support,  sum_v, df[i])
	}
	return df, count
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
		fetchLineNRGBA64(flipXY, column, x, src)
		return

	}
	dy := src.Bounds().Min.Y
	dx := src.Bounds().Min.X
	var r, g, b, a uint32
	for y := 0; y != len(column); y++ {
		if flipXY {
			r, g, b, a = src.At(y+dx, x+dy).RGBA()
		} else {
			r, g, b, a = src.At(x+dx, y+dy).RGBA()
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
			dst.SetNRGBA64(y+dx, x+dy, dst_nrgba)
		} else {
			dst.SetNRGBA64(x+dx, y+dy, dst_nrgba)
		}
	}
}

// Resample axis..
func resampleAxisNRGBA64(axis axisSwitch, keepAlive func(int) bool, dst *image.NRGBA64, src image.Image, f [][]kvPair) {
	flip := axis != yAxis

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

	// This assertion is only triggered if the dst image
	// src image don't have the correct sizes. This can
	// only happen from ResizeToChannelWithFilter right now
	// and thus we keep the ugly panic to make sure we do
	// use this function correctly.
	if dst_max_x-dst_min_x != xsize {
		panic("Unfiltered axis must have preserved size.")
	}

	src_column := make([]f32RGBA, ysize)
	dst_column := make([]f32RGBA, dst_max_y-dst_min_y)

	for x := dst_min_x; x != dst_max_x; x++ {
		var opCount int
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
			opCount += len(f[y_i])
			y_i++
		}
		putLineNRGBA64(flip, dst_column, x, dst)
		if !keepAlive(opCount) {
			return
		}
	}
}
