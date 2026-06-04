package engine

type Scorer interface {
	ComputeImportanceMap(target []float32, w, h int) []float32
}

func clamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ComputeImportanceMap weights scoring toward edges, alpha boundaries, saturated detail, and linework
func ComputeImportanceMap(target []float32, w, h int) []float32 {
	total := w * h
	if total == 0 {
		return nil
	}

	luma := make([]float32, total)
	alpha := make([]float32, total)
	saturation := make([]float32, total)

	for i := 0; i < total; i++ {
		idx := i * 4
		r := target[idx+0]
		g := target[idx+1]
		b := target[idx+2]
		a := target[idx+3]

		alpha[i] = a
		luma[i] = r*0.299 + g*0.587 + b*0.114

		// Saturation = max(r,g,b) - min(r,g,b)
		maxC := r
		if g > maxC {
			maxC = g
		}
		if b > maxC {
			maxC = b
		}
		minC := r
		if g < minC {
			minC = g
		}
		if b < minC {
			minC = b
		}
		saturation[i] = maxC - minC
	}

	edge := make([]float32, total)
	alphaEdge := make([]float32, total)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			var gx, gy float32
			var agx, agy float32

			if x > 0 {
				gx = luma[idx] - luma[idx-1]
				if gx < 0 {
					gx = -gx
				}
				agx = alpha[idx] - alpha[idx-1]
				if agx < 0 {
					agx = -agx
				}
			}
			if y > 0 {
				gy = luma[idx] - luma[idx-w]
				if gy < 0 {
					gy = -gy
				}
				agy = alpha[idx] - alpha[idx-w]
				if agy < 0 {
					agy = -agy
				}
			}

			if gx > gy {
				edge[idx] = gx
			} else {
				edge[idx] = gy
			}

			if agx > agy {
				alphaEdge[idx] = agx
			} else {
				alphaEdge[idx] = agy
			}
		}
	}

	importance := make([]float32, total)
	for i := 0; i < total; i++ {
		l := luma[i]
		a := alpha[i]
		sat := saturation[i]

		linework := clamp((0.48-l)/0.48, 0, 1) * clamp(sat*1.35, 0, 1) * a
		highlights := clamp((l-0.78)/0.22, 0, 1) * clamp(sat*1.15, 0, 1) * a

		var visible float32 = 0.55
		if a > 0.02 {
			visible = 1.0
		}

		imp := 1.0 +
			clamp(edge[i]*9.0, 0, 2.6) +
			clamp(alphaEdge[i]*7.5, 0, 2.8) +
			clamp(sat*0.55, 0, 0.75)*a +
			linework*1.35 +
			highlights*0.70

		importance[i] = imp * visible
	}

	// 3x3 Dilation to protect thin details
	dilated := make([]float32, total)
	copy(dilated, importance)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			maxVal := importance[idx]

			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 {
					ny = 0
				}
				if ny >= h {
					ny = h - 1
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 {
						nx = 0
					}
					if nx >= w {
						nx = w - 1
					}

					val := importance[ny*w+nx] * 0.92
					if val > maxVal {
						maxVal = val
					}
				}
			}
			dilated[idx] = clamp(maxVal, 0.55, 5.25)
		}
	}

	return dilated
}
