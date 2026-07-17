package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"

	"github.com/HugoSmits86/nativewebp"
	lossywebp "github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"
)

var (
	ErrImageEmpty        = errors.New("image is empty")
	ErrImageFormat       = errors.New("unsupported image format")
	ErrImageDimensions   = errors.New("invalid image dimensions")
	ErrImageWebPEncoding = errors.New("encode image as webp")
)

func ConvertImageToWebP(content []byte, maxDimension int, quality float32) ([]byte, int, int, error) {
	if len(content) == 0 {
		return nil, 0, 0, ErrImageEmpty
	}

	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		img, err = nativewebp.Decode(bytes.NewReader(content))
		if err != nil {
			return nil, 0, 0, ErrImageFormat
		}
	}
	img = resizeImageToMaxDimension(img, maxDimension)
	width, height := imageDimensions(img)
	if width <= 0 || height <= 0 {
		return nil, 0, 0, ErrImageDimensions
	}

	var buffer bytes.Buffer
	if err := lossywebp.Encode(&buffer, img, &lossywebp.Options{Quality: quality}); err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %v", ErrImageWebPEncoding, err)
	}

	return buffer.Bytes(), width, height, nil
}

func resizeImageToMaxDimension(img image.Image, maxDimension int) image.Image {
	width, height := imageDimensions(img)
	if width <= 0 || height <= 0 || maxDimension <= 0 || (width <= maxDimension && height <= maxDimension) {
		return img
	}

	scale := float64(maxDimension) / float64(max(width, height))
	targetWidth := max(1, int(math.Round(float64(width)*scale)))
	targetHeight := max(1, int(math.Round(float64(height)*scale)))
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(target, target.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	return target
}

func imageDimensions(img image.Image) (int, int) {
	if img == nil {
		return 0, 0
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy()
}
