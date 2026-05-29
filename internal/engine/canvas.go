package engine

import (
	"image"
	"math"

	"forza-painter-geometrize-go/internal/model"
	"golang.org/x/image/draw"
)

// ScaleImageFloat32 scales a float32 RGBA buffer
func ScaleImageFloat32(src []float32, srcW, srcH, scaleSize int) ([]float32, int, int) {
	maxDim := srcW
	if srcH > maxDim {
		maxDim = srcH
	}
	if scaleSize <= 0 || maxDim <= scaleSize {
		return src, srcW, srcH
	}
	scale := float64(scaleSize) / float64(maxDim)
	dstW := int(math.Round(float64(srcW) * scale))
	dstH := int(math.Round(float64(srcH) * scale))
	if dstW < 1 { dstW = 1 }
	if dstH < 1 { dstH = 1 }

	imgSrc := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
	for i := 0; i < srcW*srcH; i++ {
		imgSrc.Pix[i*4+0] = uint8(clampF(src[i*4+0]) * 255)
		imgSrc.Pix[i*4+1] = uint8(clampF(src[i*4+1]) * 255)
		imgSrc.Pix[i*4+2] = uint8(clampF(src[i*4+2]) * 255)
		imgSrc.Pix[i*4+3] = uint8(clampF(src[i*4+3]) * 255)
	}

	imgDst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(imgDst, imgDst.Bounds(), imgSrc, imgSrc.Bounds(), draw.Over, nil)

	dst := make([]float32, dstW*dstH*4)
	for i := 0; i < dstW*dstH; i++ {
		dst[i*4+0] = float32(imgDst.Pix[i*4+0]) / 255.0
		dst[i*4+1] = float32(imgDst.Pix[i*4+1]) / 255.0
		dst[i*4+2] = float32(imgDst.Pix[i*4+2]) / 255.0
		dst[i*4+3] = float32(imgDst.Pix[i*4+3]) / 255.0
	}
	return dst, dstW, dstH
}

func clampF(v float32) float32 {
	if v < 0 { return 0 }
	if v > 1 { return 1 }
	return v
}

// ScaleMask scales a uint8 mask buffer (Nearest Neighbor)
func ScaleMask(src []uint8, srcW, srcH, dstW, dstH int) []uint8 {
	if srcW == dstW && srcH == dstH {
		return src
	}
	dst := make([]uint8, dstW*dstH)
	for y := 0; y < dstH; y++ {
		sy := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			sx := x * srcW / dstW
			dst[y*dstW+x] = src[sy*srcW+sx]
		}
	}
	return dst
}

// RescaleShapeCoords scales the shape parameters from pass resolution back to original resolution
func RescaleShapeCoords(s model.Shape, scaleW, scaleH, origW, origH int) model.Shape {
	scaleX := float64(origW) / float64(scaleW)
	scaleY := float64(origH) / float64(scaleH)

	scaled := s
	scaled.Data = make([]float64, len(s.Data))
	copy(scaled.Data, s.Data)

	scaled.Data[0] = s.Data[0] * scaleX
	scaled.Data[1] = s.Data[1] * scaleY
	scaled.Data[2] = float64(snapToValidRX(float32(s.Data[2] * scaleX)))
	scaled.Data[3] = float64(snapToValidRX(float32(s.Data[3] * scaleY)))
	// Angle (Data[4]) remains unchanged

	return scaled
}

// DefaultPassConfigs provides the default 3-pass settings
func DefaultPassConfigs(totalShapes int) []model.PassConfig {
	p1Count := int(float64(totalShapes) * 0.20)
	p2Count := int(float64(totalShapes) * 0.35)
	p3Count := totalShapes - p1Count - p2Count

	return []model.PassConfig{
		{
			Name:       "Pass1-LowFreq",
			ScaleSize:  64,
			ShapeCount: p1Count,
			MinRadius:  0.08,
			MaxRadius:  0.45,
			EdgeWeight: 0.0,
		},
		{
			Name:       "Pass2-MidFreq",
			ScaleSize:  128,
			ShapeCount: p2Count,
			MinRadius:  0.03,
			MaxRadius:  0.15,
			EdgeWeight: 2.0,
		},
		{
			Name:       "Pass3-HighFreq",
			ScaleSize:  0, // Original
			ShapeCount: p3Count,
			MinRadius:  0.005,
			MaxRadius:  0.04,
			EdgeWeight: 4.0,
		},
	}
}
