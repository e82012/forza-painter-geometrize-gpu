package imageutil

import (
	"math"
)

type FringeReport struct {
	Enabled         bool
	Changed         bool
	RemovedPixels   int
	RemovedFraction float64
}

// RemoveAlphaFringe eliminates isolated low-alpha fringe/halo noise
func RemoveAlphaFringe(target []float32, w, h int) ([]float32, FringeReport) {
	total := w * h
	if total == 0 {
		return target, FringeReport{Enabled: false}
	}

	// Step 1: Detect core and hard_drop
	hardDrop := make([]bool, total)
	core := make([]bool, total)
	hasCore := false

	for i := 0; i < total; i++ {
		alpha := target[i*4+3]
		if alpha <= 16.0/255.0 {
			hardDrop[i] = true
		}
		if alpha >= 96.0/255.0 {
			core[i] = true
			hasCore = true
		}
	}

	nearCore := make([]bool, total)
	if hasCore {
		// Circular structural offsets for 7x7 ellipse dilation
		// dx^2 + dy^2 <= 10 covers radius ~3 (7x7)
		offsets := []struct{ dx, dy int }{}
		for dy := -3; dy <= 3; dy++ {
			for dx := -3; dx <= 3; dx++ {
				if dx*dx+dy*dy <= 10 {
					offsets = append(offsets, struct{ dx, dy int }{dx, dy})
				}
			}
		}

		// Perform dilation
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if core[y*w+x] {
					for _, off := range offsets {
						nx := x + off.dx
						ny := y + off.dy
						if nx >= 0 && nx < w && ny >= 0 && ny < h {
							nearCore[ny*w+nx] = true
						}
					}
				}
			}
		}
	}

	// Compute soft haze & final drop mask
	drop := make([]bool, total)
	removedCount := 0
	for i := 0; i < total; i++ {
		alpha := target[i*4+3]
		isSoftHaze := false
		if hasCore {
			isSoftHaze = (alpha < 48.0/255.0) && !nearCore[i]
		}
		drop[i] = hardDrop[i] || isSoftHaze

		if drop[i] && alpha > 0 {
			removedCount++
		}
	}

	if removedCount == 0 {
		return target, FringeReport{
			Enabled:         true,
			Changed:         false,
			RemovedPixels:   0,
			RemovedFraction: 0.0,
		}
	}

	cleaned := make([]float32, len(target))
	copy(cleaned, target)

	for i := 0; i < total; i++ {
		if drop[i] {
			idx := i * 4
			cleaned[idx+0] = 0.0
			cleaned[idx+1] = 0.0
			cleaned[idx+2] = 0.0
			cleaned[idx+3] = 0.0
		}
	}

	return cleaned, FringeReport{
		Enabled:         true,
		Changed:         true,
		RemovedPixels:   removedCount,
		RemovedFraction: float64(removedCount) / float64(total),
	}
}

// ApplyLogoHardEdges snaps visible alpha to opaque and fills semi-transparent edge pixels using closest solid pixel color
func ApplyLogoHardEdges(target []float32, w, h int, alphaThreshold float32) []float32 {
	total := w * h
	if total == 0 {
		return target
	}

	visible := make([]bool, total)
	softVisible := make([]bool, total)
	solid := make([]bool, total)
	hasSoftVisible := false
	hasSolid := false

	for i := 0; i < total; i++ {
		alpha := target[i*4+3]
		if alpha >= alphaThreshold {
			visible[i] = true
			if alpha < 245.0/255.0 {
				softVisible[i] = true
				hasSoftVisible = true
			} else {
				solid[i] = true
				hasSolid = true
			}
		}
	}

	// Output buffer
	out := make([]float32, len(target))
	copy(out, target)

	// If we have soft visible pixels and solid colors, find the closest solid pixel to fill the soft border colors
	if hasSoftVisible && hasSolid {
		nearestSolid := make([]int, total)
		for i := range nearestSolid {
			nearestSolid[i] = -1
		}

		queue := make([]int, 0, total)
		for i := 0; i < total; i++ {
			if solid[i] {
				nearestSolid[i] = i
				queue = append(queue, i)
			}
		}

		// BFS to compute closest solid pixel index for all pixels
		head := 0
		dirs := []int{-1, 1, -w, w} // 4-way connectivity
		for head < len(queue) {
			curr := queue[head]
			head++

			cx := curr % w
			cy := curr / w
			sourceSolid := nearestSolid[curr]

			for _, d := range dirs {
				next := curr + d
				if next >= 0 && next < total {
					// Bounds check for left/right wrapping
					nx := next % w
					ny := next / w
					if math.Abs(float64(cx-nx)) > 1 || math.Abs(float64(cy-ny)) > 1 {
						continue
					}
					if nearestSolid[next] == -1 {
						nearestSolid[next] = sourceSolid
						queue = append(queue, next)
					}
				}
			}
		}

		// Fill colors for soft visible pixels
		for i := 0; i < total; i++ {
			if softVisible[i] {
				ref := nearestSolid[i]
				if ref != -1 {
					out[i*4+0] = target[ref*4+0]
					out[i*4+1] = target[ref*4+1]
					out[i*4+2] = target[ref*4+2]
				}
			}
		}
	}

	// Snap alpha channel
	for i := 0; i < total; i++ {
		if visible[i] {
			out[i*4+3] = 1.0
		} else {
			out[i*4+0] = 0.0
			out[i*4+1] = 0.0
			out[i*4+2] = 0.0
			out[i*4+3] = 0.0
		}
	}

	return out
}

// Color conversion helpers (sRGB <-> XYZ <-> LAB)
func pivotRGB(n float32) float32 {
	if n > 0.04045 {
		return float32(math.Pow(float64((n+0.055)/1.055), 2.4))
	}
	return n / 12.92
}

func pivotRGBInv(n float32) float32 {
	if n > 0.0031308 {
		return float32(1.055*math.Pow(float64(n), 1.0/2.4) - 0.055)
	}
	return n * 12.92
}

func pivotXYZ(n float32) float32 {
	if n > 0.008856 {
		return float32(math.Pow(float64(n), 1.0/3.0))
	}
	return 7.787*n + 16.0/116.0
}

func pivotXYZInv(n float32) float32 {
	n3 := n * n * n
	if n3 > 0.008856 {
		return n3
	}
	return (n - 16.0/116.0) / 7.787
}

const (
	refX float32 = 0.95047
	refY float32 = 1.00000
	refZ float32 = 1.08883
)

func rgbToLab(rIn, gIn, bIn float32) (float32, float32, float32) {
	// RGB -> XYZ
	rP := pivotRGB(rIn)
	gP := pivotRGB(gIn)
	bP := pivotRGB(bIn)
	x := rP*0.4124 + gP*0.3576 + bP*0.1805
	y := rP*0.2126 + gP*0.7152 + bP*0.0722
	z := rP*0.0193 + gP*0.1192 + bP*0.9505

	// XYZ -> LAB (L mapped to 0..255 like OpenCV)
	x = pivotXYZ(x / refX)
	y = pivotXYZ(y / refY)
	z = pivotXYZ(z / refZ)

	lNorm := 116.0*y - 16.0
	l := lNorm * 255.0 / 100.0 // Map 0..100 to 0..255
	a := 500.0 * (x - y)       // standard A [-128, 127]
	b := 200.0 * (y - z)       // standard B [-128, 127]
	return l, a, b
}

func labToRgb(lIn, aIn, bIn float32) (float32, float32, float32) {
	// LAB -> XYZ
	lNorm := lIn * 100.0 / 255.0 // Unmap 0..255 to 0..100
	y := (lNorm + 16.0) / 116.0
	x := aIn/500.0 + y
	z := y - bIn/200.0

	x = pivotXYZInv(x) * refX
	y = pivotXYZInv(y) * refY
	z = pivotXYZInv(z) * refZ

	// XYZ -> RGB
	r := x*3.2406 + y*-1.5372 + z*-0.4986
	g := x*-0.9689 + y*1.8758 + z*0.0415
	b := x*0.0557 + y*-0.2040 + z*1.0570

	r = pivotRGBInv(r)
	g = pivotRGBInv(g)
	b = pivotRGBInv(b)

	return r, g, b
}

// ApplyLumaBands applies luminance banding quantization to smooth out gradients
func ApplyLumaBands(target []float32, w, h int, levels float32) []float32 {
	total := w * h
	if total == 0 {
		return target
	}

	luma := make([]float32, total)
	aChan := make([]float32, total)
	bChan := make([]float32, total)
	alpha := make([]float32, total)

	for i := 0; i < total; i++ {
		idx := i * 4
		l, a, b := rgbToLab(target[idx+0], target[idx+1], target[idx+2])
		luma[i] = l
		aChan[i] = a
		bChan[i] = b
		alpha[i] = target[idx+3]
	}

	// 1. Quantize Luma
	step := 256.0 / levels
	lq := make([]float32, total)
	for i := 0; i < total; i++ {
		val := float64(luma[i])
		lq[i] = float32(math.Floor(val/float64(step))*float64(step) + float64(step)*0.5)
	}

	// 2. Gaussian Blur (approximate with 5x5 separable kernel, sigma=1.1)
	// Kernel values for sigma = 1.1: [0.055, 0.244, 0.402, 0.244, 0.055] normalized
	k := []float32{0.054489, 0.244201, 0.40261, 0.244201, 0.054489}
	// normalize kernel
	var sumK float32
	for _, v := range k {
		sumK += v
	}
	for i := range k {
		k[i] /= sumK
	}

	temp := make([]float32, total)
	// Horizontal pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float32
			var weight float32
			for dx := -2; dx <= 2; dx++ {
				nx := x + dx
				if nx >= 0 && nx < w {
					sum += luma[y*w+nx] * k[dx+2]
					weight += k[dx+2]
				}
			}
			temp[y*w+x] = sum / weight
		}
	}

	blur := make([]float32, total)
	// Vertical pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float32
			var weight float32
			for dy := -2; dy <= 2; dy++ {
				ny := y + dy
				if ny >= 0 && ny < h {
					sum += temp[ny*w+x] * k[dy+2]
					weight += k[dy+2]
				}
			}
			blur[y*w+x] = sum / weight
		}
	}

	// 3. Sobel Edge Detection on Blurred Luma
	edge := make([]float32, total)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			tl := blur[(y-1)*w+(x-1)]
			tc := blur[(y-1)*w+x]
			tr := blur[(y-1)*w+(x+1)]
			ml := blur[y*w+(x-1)]
			mr := blur[y*w+(x+1)]
			bl := blur[(y+1)*w+(x-1)]
			bc := blur[(y+1)*w+x]
			br := blur[(y+1)*w+(x+1)]

			gx := -tl + tr - 2.0*ml + 2.0*mr - bl + br
			gy := -tl - 2.0*tc - tr + bl + 2.0*bc + br

			edge[y*w+x] = float32(math.Sqrt(float64(gx*gx + gy*gy)))
		}
	}

	// 4. Combine Luma Bands with original Luma using edge weight
	out := make([]float32, len(target))
	for i := 0; i < total; i++ {
		e := edge[i]
		// Map Sobel magnitude to [0..1] range, typical thresholding: (edge - 3.0)/18.0
		eNorm := (e - 3.0) / 18.0
		if eNorm < 0 {
			eNorm = 0
		}
		if eNorm > 1 {
			eNorm = 1
		}

		bandWeight := 0.16 + eNorm*0.34
		lOut := lq[i]*bandWeight + luma[i]*(1.0-bandWeight)
		// OpenCV range contrast adjustment
		lOut = (lOut-128.0)*1.005 + 128.0
		if lOut < 0 {
			lOut = 0
		}
		if lOut > 255 {
			lOut = 255
		}

		r, g, b := labToRgb(lOut, aChan[i], bChan[i])
		idx := i * 4
		out[idx+0] = r
		out[idx+1] = g
		out[idx+2] = b
		out[idx+3] = alpha[i]
	}

	return out
}

type ImageProfile struct {
	AlphaCoverage       float64
	EdgeDensity         float64
	LumaStd             float64
	WhiteFraction       float64
	Category            string
	RecommendedLumaPrep string
	Recommendation      string
}

// ProfileImage analyzes the source target image and returns styling profile recommendations
func ProfileImage(target []float32, w, h int) ImageProfile {
	total := w * h
	if total == 0 {
		return ImageProfile{Category: "empty", Recommendation: "No pixels"}
	}

	visibleCount := 0
	var sumLuma float64
	var visibleLumas []float64

	lumaMasked := make([]float32, total)
	alphaNorm := make([]float32, total)

	for i := 0; i < total; i++ {
		r := target[i*4+0]
		g := target[i*4+1]
		b := target[i*4+2]
		a := target[i*4+3]

		l := float64(r)*0.2126 + float64(g)*0.7152 + float64(b)*0.0722
		lumaMasked[i] = float32(l * float64(a))
		alphaNorm[i] = a

		if a > 16.0/255.0 {
			visibleCount++
			sumLuma += l
			visibleLumas = append(visibleLumas, l*255.0)
		}
	}

	alphaCoverage := float64(visibleCount) / float64(total)
	if visibleCount == 0 {
		return ImageProfile{
			AlphaCoverage:       0.0,
			Category:            "empty_alpha",
			RecommendedLumaPrep: "none",
			Recommendation:      "No visible pixels were detected.",
		}
	}

	meanLuma := sumLuma * 255.0 / float64(visibleCount)
	var sumSqDiff float64
	for _, val := range visibleLumas {
		diff := val - meanLuma
		sumSqDiff += diff * diff
	}
	lumaStd := math.Sqrt(sumSqDiff / float64(visibleCount))

	whiteCount := 0
	for i := 0; i < total; i++ {
		r := target[i*4+0]
		g := target[i*4+1]
		b := target[i*4+2]
		a := target[i*4+3]
		l := float64(r)*0.2126 + float64(g)*0.7152 + float64(b)*0.0722

		if a > 16.0/255.0 {
			if l*255.0 > 232.0 && a*255.0 > 128.0 {
				whiteCount++
			}
		}
	}
	whiteFraction := float64(whiteCount) / float64(visibleCount)

	edgeDensityCount := 0
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			tl := lumaMasked[(y-1)*w+(x-1)]
			tc := lumaMasked[(y-1)*w+x]
			tr := lumaMasked[(y-1)*w+(x+1)]
			ml := lumaMasked[y*w+(x-1)]
			mr := lumaMasked[y*w+(x+1)]
			bl := lumaMasked[(y+1)*w+(x-1)]
			bc := lumaMasked[(y+1)*w+x]
			br := lumaMasked[(y+1)*w+(x+1)]

			gx := -tl + tr - 2.0*ml + 2.0*mr - bl + br
			gy := -tl - 2.0*tc - tr + bl + 2.0*bc + br
			edgeLuma := math.Sqrt(float64(gx*gx + gy*gy)) * 255.0

			tlA := alphaNorm[(y-1)*w+(x-1)]
			tcA := alphaNorm[(y-1)*w+x]
			trA := alphaNorm[(y-1)*w+(x+1)]
			mlA := alphaNorm[y*w+(x-1)]
			mrA := alphaNorm[y*w+(x+1)]
			blA := alphaNorm[(y+1)*w+(x-1)]
			bcA := alphaNorm[(y+1)*w+x]
			brA := alphaNorm[(y+1)*w+(x+1)]

			gxA := -tlA + trA - 2.0*mlA + 2.0*mrA - blA + brA
			gyA := -tlA - 2.0*tcA - trA + blA + 2.0*bcA + brA
			edgeAlpha := math.Sqrt(float64(gxA*gxA + gyA*gyA)) * 255.0

			totalEdge := edgeLuma + edgeAlpha

			aVal := target[(y*w+x)*4+3]
			if aVal > 16.0/255.0 {
				if totalEdge > 35.0 {
					edgeDensityCount++
				}
			}
		}
	}
	edgeDensity := float64(edgeDensityCount) / float64(visibleCount)

	var category, recommendedLuma, recommendation string
	if alphaCoverage < 0.18 && whiteFraction > 0.45 {
		category = "sparse_white_line_art"
		recommendedLuma = "none"
		recommendation = "Sparse white transparent line art: use edge-biased shapes, enough layers, and usually leave Luma Prep off."
	} else if edgeDensity >= 0.34 && lumaStd >= 55.0 {
		category = "flat_crisp_livery"
		recommendedLuma = "luma_bands"
		recommendation = "Flat crisp livery art: edge-biased shapes and Luma Prep usually preserve borders and broad color regions best."
	} else if alphaCoverage < 0.70 && edgeDensity < 0.30 && lumaStd < 65.0 {
		category = "soft_gradient_character"
		recommendedLuma = "none"
		recommendation = "Soft transparent character art: soft-detail or smart-detail weighting without Luma Prep usually avoids posterized gradients, hard rectangle blocks, and over-smoothed hair."
	} else {
		category = "general_art"
		recommendedLuma = "none"
		recommendation = "General art: smart-detail weighting without Luma Prep is the safer default; enable Luma Prep manually for flat logo/livery sources."
	}

	return ImageProfile{
		AlphaCoverage:       alphaCoverage,
		EdgeDensity:         edgeDensity,
		LumaStd:             lumaStd,
		WhiteFraction:       whiteFraction,
		Category:            category,
		RecommendedLumaPrep: recommendedLuma,
		Recommendation:      recommendation,
	}
}

