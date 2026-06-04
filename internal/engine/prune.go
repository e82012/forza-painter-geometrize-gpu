package engine

import (
	"fmt"
	"math"

	"forza-painter-geometrize-go/internal/model"
)

// PruneShapes performs contribution-based pruning on the generated shapes.
func PruneShapes(target []float32, mask []uint8, w, h int, shapes []model.Shape, threshold float64, backgroundRGBA [4]uint8, hasTransparency bool) []model.Shape {
	if len(shapes) <= 2 {
		return shapes
	}

	n := len(shapes)
	fmt.Printf("[Pruning] Starting CPU contribution-based pruning on %d shapes with threshold %g...\n", n, threshold)

	topR := make([]float32, w*h)
	topG := make([]float32, w*h)
	topB := make([]float32, w*h)
	topA := make([]float32, w*h)

	underR := make([]float32, w*h)
	underG := make([]float32, w*h)
	underB := make([]float32, w*h)
	underA := make([]float32, w*h)

	topIdx := make([]int, w*h)
	for i := range topIdx {
		topIdx[i] = -1
	}

	// Initialize background
	bgR := float32(backgroundRGBA[0]) / 255.0
	bgG := float32(backgroundRGBA[1]) / 255.0
	bgB := float32(backgroundRGBA[2]) / 255.0
	bgA := float32(1.0)
	if hasTransparency {
		bgA = 0.0
		bgR = 0.0
		bgG = 0.0
		bgB = 0.0
	}

	for i := 0; i < w*h; i++ {
		topR[i] = bgR
		topG[i] = bgG
		topB[i] = bgB
		topA[i] = bgA

		underR[i] = bgR
		underG[i] = bgG
		underB[i] = bgB
		underA[i] = bgA
	}

	fmt.Printf("[Pruning] Step 1/3: Rendering shapes on CPU...\n")
	// Render all shapes on CPU
	for idx := 1; idx < n; idx++ {
		if idx%200 == 0 || idx == n-1 {
			fmt.Printf("[Pruning]   Rendered %d/%d shapes...\n", idx, n-1)
		}
		s := shapes[idx]
		cx := s.Data[0]
		cy := s.Data[1]
		rx := s.Data[2] / 1.0125
		ry := s.Data[3] / 1.0125
		thetaRad := s.Data[4] * math.Pi / 180.0

		colorR := float32(s.Color[0]) / 255.0
		colorG := float32(s.Color[1]) / 255.0
		colorB := float32(s.Color[2]) / 255.0
		alpha := float32(s.Color[3]) / 255.0

		// Bounding box
		rMax := rx
		if ry > rMax {
			rMax = ry
		}
		x0 := int(math.Floor(cx - rMax))
		x1 := int(math.Ceil(cx + rMax))
		y0 := int(math.Floor(cy - rMax))
		y1 := int(math.Ceil(cy + rMax))

		if x0 < 0 {
			x0 = 0
		}
		if x1 >= w {
			x1 = w - 1
		}
		if y0 < 0 {
			y0 = 0
		}
		if y1 >= h {
			y1 = h - 1
		}

		cosT := math.Cos(-thetaRad)
		sinT := math.Sin(-thetaRad)

		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				dx := float64(x) + 0.5 - cx
				dy := float64(y) + 0.5 - cy
				localX := dx*cosT - dy*sinT
				localY := dx*sinT + dy*cosT

				if (localX*localX)/(rx*rx)+(localY*localY)/(ry*ry) <= 1.0 {
					p := y*w + x
					// Store under state
					underR[p] = topR[p]
					underG[p] = topG[p]
					underB[p] = topB[p]
					underA[p] = topA[p]

					// Blend top state
					if alpha >= 1.0 {
						topR[p] = colorR
						topG[p] = colorG
						topB[p] = colorB
					} else if alpha > 0.0 {
						topR[p] = topR[p]*(1.0-alpha) + colorR*alpha
						topG[p] = topG[p]*(1.0-alpha) + colorG*alpha
						topB[p] = topB[p]*(1.0-alpha) + colorB*alpha
					}
					if alpha > 0.0 {
						topA[p] = topA[p] + alpha*(1.0-topA[p])
					}
					topIdx[p] = idx
				}
			}
		}
	}

	fmt.Printf("[Pruning] Step 2/3: Calculating shape contributions...\n")
	// Compute contributions
	contributions := make([]float64, n)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			if mask[p] == 0 {
				continue
			}

			targetR := target[p*4+0]
			targetG := target[p*4+1]
			targetB := target[p*4+2]
			targetA := target[p*4+3]

			rAvgTop := (targetR + topR[p]) * 0.5
			wrTop := 0.2 + 0.1*float64(rAvgTop)
			wgTop := 0.4
			wbTop := 0.3 + 0.1*float64(1.0-rAvgTop)

			drTop := float64(targetR - topR[p])
			dgTop := float64(targetG - topG[p])
			dbTop := float64(targetB - topB[p])
			daTop := float64(targetA - topA[p])

			errTop := wrTop*drTop*drTop + wgTop*dgTop*dgTop + wbTop*dbTop*dbTop + daTop*daTop

			rAvgUnder := (targetR + underR[p]) * 0.5
			wrUnder := 0.2 + 0.1*float64(rAvgUnder)
			wgUnder := 0.4
			wbUnder := 0.3 + 0.1*float64(1.0-rAvgUnder)

			drUnder := float64(targetR - underR[p])
			dgUnder := float64(targetG - underG[p])
			dbUnder := float64(targetB - underB[p])
			daUnder := float64(targetA - underA[p])

			errUnder := wrUnder*drUnder*drUnder + wgUnder*dgUnder*dgUnder + wbUnder*dbUnder*dbUnder + daUnder*daUnder

			delta := errUnder - errTop
			idx := topIdx[p]
			if idx >= 0 {
				contributions[idx] += delta
			}
		}
	}

	fmt.Printf("[Pruning] Step 3/3: Filtering low-contribution shapes...\n")
	kept := make([]model.Shape, 0, n)
	kept = append(kept, shapes[0]) // Always keep background shape

	prunedCount := 0
	for idx := 1; idx < n; idx++ {
		if contributions[idx] > threshold {
			kept = append(kept, shapes[idx])
		} else {
			prunedCount++
		}
	}

	fmt.Printf("[Pruning] Completed! CPU-based pruning removed %d shapes out of %d. Final shape count: %d\n", prunedCount, n, len(kept))
	return kept
}
