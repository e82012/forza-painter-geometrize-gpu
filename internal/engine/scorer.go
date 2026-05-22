package engine

import (
	"math"
)

// ComputeEdgeMap 對輸入圖像計算 Sobel 邊緣強度圖
// 輸入：target []float32，長度為 w*h*4（RGBA 格式，值域 0~1）
// 輸出：edgeMap []float32，長度為 w*h，值域 [0.0, 1.0]
func ComputeEdgeMap(target []float32, w, h int) []float32 {
	gray := make([]float32, w*h)
	for i := 0; i < w*h; i++ {
		r := target[i*4+0]
		g := target[i*4+1]
		b := target[i*4+2]
		gray[i] = 0.299*r + 0.587*g + 0.114*b
	}

	edgeMap := make([]float32, w*h)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			tl := gray[(y-1)*w+(x-1)]
			tc := gray[(y-1)*w+x]
			tr := gray[(y-1)*w+(x+1)]
			ml := gray[y*w+(x-1)]
			mr := gray[y*w+(x+1)]
			bl := gray[(y+1)*w+(x-1)]
			bc := gray[(y+1)*w+x]
			br := gray[(y+1)*w+(x+1)]

			gx := -tl + tr - 2*ml + 2*mr - bl + br
			gy := -tl - 2*tc - tr + bl + 2*bc + br

			magnitude := float32(math.Sqrt(float64(gx*gx + gy*gy)))
			edgeMap[y*w+x] = magnitude
		}
	}

	var maxVal float32 = 1e-8
	for _, v := range edgeMap {
		if v > maxVal {
			maxVal = v
		}
	}
	for i := range edgeMap {
		edgeMap[i] /= maxVal
	}

	return edgeMap
}
