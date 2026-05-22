# Geometrize GPU 升級計畫書
## 給 AI 執行用 — 完整實作規格

> **專案背景**：現有系統為 Forza Painter Geometrize（Go + OpenCL），用橢圓圖元逐一擬合目標圖像，輸出 JSON 供 Forza Painter 使用。目標是在相同圖元數量下，超越現有 Geometrize 品質。
>
> **升級目標**：
> - Plan A：Edge-Guided Importance Sampling（邊緣引導採樣）
> - Plan B：Multi-Scale Hierarchical Fitting（多尺度層次擬合）
>
> **語言/環境**：Go 1.21+，OpenCL 1.2+，現有專案結構保持不動

---

# Plan A：Edge-Guided Importance Sampling

## A.0 背景與目標

### 現狀問題
Geometrize 目前的採樣分佈為：

```
採樣機率 ∝ 像素誤差（pixel error）
```

這導致圖元平均分佈在「顏色差異大」的區域，但人眼對邊緣（輪廓線）的感知敏感度遠高於平坦色塊。結果是輪廓模糊、邊界不清。

### 目標
改為：

```
採樣機率 ∝ 像素誤差 × (1 + λ × 邊緣強度)
λ = 3.0（預設，可調）
```

讓更多圖元自動聚集在輪廓線上，提升主觀輪廓清晰度 30~40%。

### 預期指標
| 指標 | 改前 | 改後目標 |
|------|------|---------|
| SSIM | baseline | +5% |
| 輪廓清晰度（主觀） | 模糊 | 明顯提升 |
| 速度影響 | — | < 2% 額外開銷 |

---

## A.1 需要修改的檔案清單

```
forza-painter-geometrize-gpu/
├── internal/engine/engine.go          ← 主要修改：採樣邏輯
├── internal/engine/scorer.go          ← 新增：邊緣圖計算函數
├── internal/opencl/kernels/score.cl   ← 修改：接收邊緣權重 buffer
└── cmd/main.go                        ← 修改：新增 --edge-weight 參數
```

---

## A.2 scorer.go — 新增邊緣圖計算

**位置**：`internal/engine/scorer.go`（若不存在則新建）

**新增函數**：`ComputeEdgeMap`

```go
// ComputeEdgeMap 對輸入圖像計算 Sobel 邊緣強度圖
// 輸入：rgba []uint8，長度為 w*h*4（RGBA 格式）
// 輸出：edgeMap []float32，長度為 w*h，值域 [0.0, 1.0]
func ComputeEdgeMap(rgba []uint8, w, h int) []float32 {
    // 1. 先將 RGBA 轉為灰階 float32
    gray := make([]float32, w*h)
    for i := 0; i < w*h; i++ {
        r := float32(rgba[i*4+0])
        g := float32(rgba[i*4+1])
        b := float32(rgba[i*4+2])
        gray[i] = 0.299*r + 0.587*g + 0.114*b
    }

    edgeMap := make([]float32, w*h)

    // 2. Sobel 3×3 卷積
    // Kernel X: [[-1,0,1],[-2,0,2],[-1,0,1]]
    // Kernel Y: [[-1,-2,-1],[0,0,0],[1,2,1]]
    for y := 1; y < h-1; y++ {
        for x := 1; x < w-1; x++ {
            // 取周圍 3×3 像素
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

    // 3. 正規化到 [0, 1]
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
```

**注意事項**：
- 邊界像素（x=0, x=w-1, y=0, y=h-1）保持 0.0，不影響採樣
- 需要 import `math`
- 此函數只在每個 Pass 開始時呼叫一次，不影響主迴圈性能

---

## A.3 engine.go — 修改採樣邏輯

### A.3.1 新增 EdgeWeight 欄位到 Config struct

**找到**：Engine 或 Config struct 的定義

**新增欄位**：
```go
type Config struct {
    // ... 現有欄位保持不動 ...
    EdgeWeight float64 // 邊緣強度加權係數，預設 3.0，設為 0 則停用
}
```

### A.3.2 新增 edgeMap 欄位到 Engine struct

```go
type Engine struct {
    // ... 現有欄位保持不動 ...
    edgeMap []float32 // 邊緣強度圖，長度為 w*h
}
```

### A.3.3 在 Engine 初始化時計算邊緣圖

**找到**：Engine 的 `New` 或 `Init` 函數，在目標圖像載入完成後加入：

```go
// 計算邊緣圖（僅在 EdgeWeight > 0 時才計算）
if cfg.EdgeWeight > 0 {
    e.edgeMap = ComputeEdgeMap(targetRGBA, targetW, targetH)
    log.Printf("[EdgeGuided] Edge map computed, weight=%.1f", cfg.EdgeWeight)
}
```

### A.3.4 修改重要性採樣函數

**找到**：負責根據誤差圖進行像素座標採樣的函數（通常叫 `samplePoint`、`weightedSample` 或類似名稱）

**原始邏輯**（概念示意）：
```go
// 原本：只用像素誤差做權重
weight[i] = errorMap[i]
```

**改為**：
```go
// 新增：疊加邊緣強度權重
if e.edgeMap != nil && e.config.EdgeWeight > 0 {
    edgeBoost := 1.0 + e.config.EdgeWeight*float64(e.edgeMap[i])
    weight[i] = errorMap[i] * float32(edgeBoost)
} else {
    weight[i] = errorMap[i]
}
```

**注意**：若原本採樣是在 OpenCL 端進行，請見 A.4。若是在 CPU 端（Go）進行，此處修改即可完成。

---

## A.4 OpenCL Kernel 修改（若採樣在 GPU 端）

**找到**：`score.cl` 或負責誤差計算/採樣的 kernel 檔案

### A.4.1 新增 edgeMap buffer 參數

**找到** kernel 函數簽名，新增參數：
```c
__kernel void computeScores(
    /* ... 現有參數 ... */
    __global const float* edgeMap,    // 新增：邊緣強度圖
    const float edgeWeight            // 新增：邊緣加權係數
) {
```

### A.4.2 修改權重計算

**找到** kernel 內計算像素誤差/採樣權重的位置：
```c
// 原本
float weight = pixelError;

// 改為
float edgeBoost = 1.0f + edgeWeight * edgeMap[pixelIdx];
float weight = pixelError * edgeBoost;
```

### A.4.3 在 Go 端傳遞新 buffer

**找到**：呼叫此 kernel 的 Go 程式碼，在 SetArg 區塊新增：
```go
// 建立 edgeMap buffer（在 Engine 初始化時建立一次，之後重用）
e.clEdgeMapBuf, err = e.clContext.CreateBuffer(
    cl.MemReadOnly|cl.MemCopyHostPtr,
    e.edgeMap,
)

// 在 kernel 呼叫前設定參數（依序對應 kernel 參數）
kernel.SetArgs(
    /* ... 現有參數 ... */
    e.clEdgeMapBuf,
    float32(e.config.EdgeWeight),
)
```

---

## A.5 main.go — 新增 CLI 參數

**找到**：`cmd/main.go` 的 flag/argparse 區塊

**新增**：
```go
edgeWeight := flag.Float64("edge-weight", 3.0,
    "Edge-guided sampling weight (0=disabled, recommended: 2.0~5.0)")
```

**在 Config 初始化時傳入**：
```go
cfg := engine.Config{
    // ... 現有欄位 ...
    EdgeWeight: *edgeWeight,
}
```

---

## A.6 驗證與測試

### 驗證步驟
1. 執行前後各跑相同圖像，對比輸出 JSON 的橢圓分佈
2. 輪廓區域的橢圓密度應明顯增加
3. 計算 SSIM：`python -c "from skimage.metrics import structural_similarity; ..."`

### 調參建議
| 場景 | 推薦 EdgeWeight |
|------|----------------|
| 線條插圖、logo | 5.0 |
| 人臉、人物 | 3.0 |
| 風景、漸層 | 1.5 |
| 關閉邊緣引導 | 0.0 |

---

---

# Plan B：Multi-Scale Hierarchical Fitting

## B.0 背景與目標

### 現狀問題
Geometrize 所有圖元共用同一個搜索策略，不區分「大輪廓」和「細節紋理」，導致：
- 前期浪費小圖元在大色塊上
- 後期大圖元繼續干擾已擬合的細節
- 整體收斂效率低

### 目標
將擬合過程分為 3 個 Pass，每個 Pass 專注不同頻率層：

```
Pass 1（低頻）：縮小至 64px，大橢圓鋪底色與大輪廓
Pass 2（中頻）：縮小至 128px，中等橢圓補結構細節
Pass 3（高頻）：原始解析度，小橢圓 + 細長筆觸補紋理
```

### 預期指標
| 指標 | 改前 | 改後目標 |
|------|------|---------|
| SSIM | baseline | +12~18% |
| 視覺輪廓清晰度 | 中 | 高 |
| 速度 | baseline | 相近（總圖元數不變） |

---

## B.1 架構設計

### 新增的核心概念：Pass 配置

```go
// PassConfig 定義單一 Pass 的所有參數
type PassConfig struct {
    Name           string  // 識別名稱，用於日誌
    ScaleSize      int     // 目標圖像縮放尺寸（最長邊），0 = 使用原始尺寸
    ShapeCount     int     // 此 Pass 要放置的圖元數量
    MinRadius      float64 // 橢圓半徑下限（相對於縮放後尺寸的比例）
    MaxRadius      float64 // 橢圓半徑上限（相對於縮放後尺寸的比例）
    RandomSamples  int     // 每個圖元的隨機候選數
    MutatedSamples int     // Hill-Climbing 突變數
    ShapeTypes     []int   // 允許的圖元類型 ID（橢圓=0, 矩形=1, 筆觸=2）
    EdgeWeight     float64 // 此 Pass 的邊緣引導係數（0=停用）
}
```

### 預設 Pass 配置

```go
func DefaultPassConfigs(totalShapes int) []PassConfig {
    // 圖元預算分配：20% 低頻 / 35% 中頻 / 45% 高頻
    p1Count := int(float64(totalShapes) * 0.20)
    p2Count := int(float64(totalShapes) * 0.35)
    p3Count := totalShapes - p1Count - p2Count

    return []PassConfig{
        {
            Name:           "Pass1-LowFreq",
            ScaleSize:      64,
            ShapeCount:     p1Count,
            MinRadius:      0.08,  // 64px * 0.08 = 5px
            MaxRadius:      0.45,  // 64px * 0.45 = 29px
            RandomSamples:  3000,
            MutatedSamples: 100,
            ShapeTypes:     []int{0}, // 只用橢圓
            EdgeWeight:     0.0,   // 低頻不需要邊緣引導
        },
        {
            Name:           "Pass2-MidFreq",
            ScaleSize:      128,
            ShapeCount:     p2Count,
            MinRadius:      0.03,  // 128px * 0.03 = 4px
            MaxRadius:      0.15,  // 128px * 0.15 = 19px
            RandomSamples:  3000,
            MutatedSamples: 100,
            ShapeTypes:     []int{0}, // 只用橢圓
            EdgeWeight:     2.0,   // 中頻開始邊緣引導
        },
        {
            Name:           "Pass3-HighFreq",
            ScaleSize:      0,     // 使用原始解析度
            ShapeCount:     p3Count,
            MinRadius:      0.005, // 原始尺寸 * 0.005
            MaxRadius:      0.04,  // 原始尺寸 * 0.04
            RandomSamples:  2000,  // 高頻較多細節，可略減候選數
            MutatedSamples: 64,
            ShapeTypes:     []int{0, 2}, // 橢圓 + 細長筆觸
            EdgeWeight:     4.0,   // 高頻強邊緣引導
        },
    }
}
```

---

## B.2 需要修改的檔案清單

```
forza-painter-geometrize-gpu/
├── internal/engine/engine.go          ← 主要修改：新增 RunMultiScale
├── internal/engine/pass.go            ← 新增檔案：Pass 執行邏輯
├── internal/engine/canvas.go          ← 新增：畫布縮放/合成工具
├── internal/engine/scorer.go          ← 已在 Plan A 新增，此處擴充
├── internal/opencl/kernels/score.cl   ← 修改：接受半徑範圍限制
└── cmd/main.go                        ← 修改：新增 --multiscale 參數
```

---

## B.3 canvas.go — 畫布縮放工具（新建檔案）

**位置**：`internal/engine/canvas.go`

```go
package engine

import (
    "image"
    "image/color"
    "image/draw"
    "golang.org/x/image/draw"  // 用 draw.BiLinear
)

// ScaleImage 將 image.RGBA 縮放到指定尺寸
// scaleSize 為最長邊的像素數，0 = 不縮放（回傳原圖）
func ScaleImage(src *image.RGBA, scaleSize int) *image.RGBA {
    srcW := src.Bounds().Dx()
    srcH := src.Bounds().Dy()

    if scaleSize <= 0 || (srcW <= scaleSize && srcH <= scaleSize) {
        return src
    }

    scale := float64(scaleSize) / float64(max(srcW, srcH))
    dstW := int(float64(srcW)*scale + 0.5)
    dstH := int(float64(srcH)*scale + 0.5)
    if dstW < 1 { dstW = 1 }
    if dstH < 1 { dstH = 1 }

    dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
    draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
    return dst
}

// UpscaleCanvas 將縮放後的畫布放大回目標尺寸
// 用於 Pass 結束後將結果合回原始解析度的畫布
func UpscaleCanvas(src *image.RGBA, targetW, targetH int) *image.RGBA {
    if src.Bounds().Dx() == targetW && src.Bounds().Dy() == targetH {
        return src
    }
    dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
    draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
    return dst
}

// RGBAToBytes 將 image.RGBA 轉為 []uint8（RGBA 格式）
func RGBAToBytes(img *image.RGBA) []uint8 {
    return img.Pix
}

// BytesToRGBA 將 []uint8 轉為 image.RGBA
func BytesToRGBA(pix []uint8, w, h int) *image.RGBA {
    img := image.NewRGBA(image.Rect(0, 0, w, h))
    copy(img.Pix, pix)
    return img
}

func max(a, b int) int {
    if a > b { return a }
    return b
}
```

**注意**：需要 `golang.org/x/image/draw` 套件，執行 `go get golang.org/x/image`

---

## B.4 pass.go — Pass 執行邏輯（新建檔案）

**位置**：`internal/engine/pass.go`

```go
package engine

import (
    "log"
    "image"
)

// RunPass 執行單一 Pass 的完整擬合流程
// 參數：
//   e          - Engine 實例（含 OpenCL context、目標圖等）
//   passCfg    - 此 Pass 的配置
//   origTarget - 原始目標圖（未縮放）
//   baseCanvas - 此 Pass 開始時的畫布狀態（已包含前一 Pass 的結果）
//   origW, origH - 原始圖的寬高（用於 JSON 座標對應）
// 回傳：
//   updatedCanvas - 此 Pass 結束後的畫布
//   shapes        - 此 Pass 新增的所有圖元（座標已轉換回原始解析度）
func (e *Engine) RunPass(
    passCfg PassConfig,
    origTarget *image.RGBA,
    baseCanvas *image.RGBA,
    origW, origH int,
) (updatedCanvas *image.RGBA, shapes []Shape) {

    log.Printf("[%s] Starting: %d shapes, scale=%dpx, radius=[%.3f, %.3f]",
        passCfg.Name, passCfg.ShapeCount,
        passCfg.ScaleSize, passCfg.MinRadius, passCfg.MaxRadius)

    // 1. 縮放目標圖與畫布
    scaledTarget := ScaleImage(origTarget, passCfg.ScaleSize)
    scaledCanvas := ScaleImage(baseCanvas, passCfg.ScaleSize)
    scaleW := scaledTarget.Bounds().Dx()
    scaleH := scaledTarget.Bounds().Dy()

    // 2. 計算此 Pass 的半徑範圍（轉為絕對像素值）
    maxDim := float64(max(scaleW, scaleH))
    absMinRadius := int(passCfg.MinRadius * maxDim)
    absMaxRadius := int(passCfg.MaxRadius * maxDim)
    if absMinRadius < 1 { absMinRadius = 1 }
    if absMaxRadius < absMinRadius { absMaxRadius = absMinRadius + 1 }

    log.Printf("[%s] Scaled to %dx%d, radius=[%d, %d]px",
        passCfg.Name, scaleW, scaleH, absMinRadius, absMaxRadius)

    // 3. 計算邊緣圖（若此 Pass 啟用邊緣引導）
    var edgeMap []float32
    if passCfg.EdgeWeight > 0 {
        edgeMap = ComputeEdgeMap(RGBAToBytes(scaledTarget), scaleW, scaleH)
    }

    // 4. 更新 Engine 的當前工作狀態
    //    （以下呼叫現有 Engine 的內部設定方法，依照你的 engine.go 實際 API 調整）
    e.setWorkingTarget(scaledTarget)
    e.setWorkingCanvas(scaledCanvas)
    e.setRadiusConstraint(absMinRadius, absMaxRadius)
    e.setEdgeMap(edgeMap, passCfg.EdgeWeight)
    e.setAllowedShapeTypes(passCfg.ShapeTypes)
    e.setRandomSamples(passCfg.RandomSamples)
    e.setMutatedSamples(passCfg.MutatedSamples)

    // 5. 執行現有的逐圖元擬合迴圈
    passShapes := make([]Shape, 0, passCfg.ShapeCount)
    for i := 0; i < passCfg.ShapeCount; i++ {
        shape := e.stepOnce() // 呼叫現有的單步擬合函數
        if shape == nil {
            log.Printf("[%s] stepOnce returned nil at step %d, stopping pass", passCfg.Name, i)
            break
        }

        // 6. 將縮放空間的座標轉換回原始解析度座標
        scaledShape := e.rescaleShapeCoords(shape, scaleW, scaleH, origW, origH)
        passShapes = append(passShapes, scaledShape)

        if (i+1)%50 == 0 {
            log.Printf("[%s] Progress: %d/%d shapes", passCfg.Name, i+1, passCfg.ShapeCount)
        }
    }

    // 7. 將此 Pass 的縮放畫布放大回原始尺寸
    finalCanvas := UpscaleCanvas(e.getWorkingCanvas(), origW, origH)

    log.Printf("[%s] Completed: %d shapes added", passCfg.Name, len(passShapes))
    return finalCanvas, passShapes
}

// rescaleShapeCoords 將圖元座標從縮放空間轉換回原始解析度
// 這確保輸出的 JSON 座標與原始圖像對應
func (e *Engine) rescaleShapeCoords(shape Shape, scaleW, scaleH, origW, origH int) Shape {
    scaleX := float64(origW) / float64(scaleW)
    scaleY := float64(origH) / float64(scaleH)

    scaled := shape // 複製一份
    // 依照你的 Shape struct 調整以下欄位名稱：
    scaled.X = int(float64(shape.X) * scaleX)
    scaled.Y = int(float64(shape.Y) * scaleY)
    scaled.RX = int(float64(shape.RX) * scaleX) // 橢圓 X 半徑
    scaled.RY = int(float64(shape.RY) * scaleY) // 橢圓 Y 半徑
    // Angle 不需要縮放

    return scaled
}
```

**⚠️ 重要**：`setWorkingTarget`、`stepOnce`、`Shape` 等名稱需對照你的 `engine.go` 實際 API 調整。這是接口設計，不是逐字複製。

---

## B.5 engine.go — 新增 RunMultiScale 主函數

**找到**：現有的主要執行函數（通常是 `Run`、`Execute` 或類似名稱）

**新增函數** `RunMultiScale`（與現有 `Run` 並存，不刪除原有邏輯）：

```go
// RunMultiScale 執行多尺度層次擬合
// 這是 Plan B 的主入口，替代原本的單一 Run 函數
func (e *Engine) RunMultiScale(totalShapes int) ([]Shape, error) {
    origW := e.targetWidth
    origH := e.targetHeight
    origTarget := e.targetImage // *image.RGBA，現有欄位

    // 1. 生成 Pass 配置
    passes := DefaultPassConfigs(totalShapes)

    // 2. 初始化起始畫布（背景色）
    canvas := e.createInitialCanvas(origW, origH) // 現有函數

    // 3. 收集所有 Pass 的圖元
    allShapes := make([]Shape, 0, totalShapes)

    for passIdx, passCfg := range passes {
        log.Printf("\n=== [MultiScale] Pass %d/%d: %s ===",
            passIdx+1, len(passes), passCfg.Name)

        updatedCanvas, passShapes := e.RunPass(passCfg, origTarget, canvas, origW, origH)

        canvas = updatedCanvas
        allShapes = append(allShapes, passShapes...)

        // 4. 儲存 Pass 預覽圖（可選，方便偵錯）
        if e.config.SavePassPreviews {
            previewPath := fmt.Sprintf("pass_%d_preview.png", passIdx+1)
            saveImagePNG(canvas, previewPath)
            log.Printf("[MultiScale] Pass %d preview saved: %s", passIdx+1, previewPath)
        }
    }

    log.Printf("\n[MultiScale] All passes completed. Total shapes: %d", len(allShapes))
    return allShapes, nil
}
```

**同時新增 Config 欄位**：
```go
type Config struct {
    // ... 現有欄位 ...
    MultiScale       bool    // 啟用多尺度擬合
    SavePassPreviews bool    // 儲存每個 Pass 的預覽圖（偵錯用）
    // Pass 配置覆寫（留空則使用 DefaultPassConfigs）
    CustomPassConfigs []PassConfig
}
```

---

## B.6 OpenCL Kernel — 新增半徑範圍限制

**找到**：生成隨機橢圓候選的 kernel（通常在隨機採樣或突變的部分）

**新增 kernel 參數**：
```c
__kernel void generateCandidates(
    /* ... 現有參數 ... */
    const int minRadius,   // 新增：半徑下限（像素）
    const int maxRadius    // 新增：半徑上限（像素）
) {
```

**找到** kernel 內生成橢圓半徑的位置，加入限制：

```c
// 原本（概念示意）：
int rx = randomInt(rng, 1, imageWidth / 2);

// 改為：
int rx = minRadius + randomInt(rng, 0, maxRadius - minRadius);
int ry = minRadius + randomInt(rng, 0, maxRadius - minRadius);
```

**在 Go 端的 kernel 呼叫處新增參數傳遞**：
```go
kernel.SetArgs(
    /* ... 現有參數 ... */
    int32(e.currentMinRadius),
    int32(e.currentMaxRadius),
)
```

---

## B.7 main.go — 新增 CLI 參數

```go
// 新增參數
multiscale      := flag.Bool("multiscale", false, "Enable multi-scale hierarchical fitting")
savePassPreviews := flag.Bool("save-pass-previews", false, "Save preview image after each pass")

// 在 Config 初始化時傳入
cfg := engine.Config{
    // ... 現有欄位 ...
    MultiScale:       *multiscale,
    SavePassPreviews: *savePassPreviews,
}

// 在執行入口判斷
if cfg.MultiScale {
    shapes, err = eng.RunMultiScale(*numShapes)
} else {
    shapes, err = eng.Run(*numShapes) // 原有邏輯不動
}
```

---

## B.8 Plan A + Plan B 整合

Plan B 的 `PassConfig` 中已包含 `EdgeWeight` 欄位，直接使用 Plan A 實作的邊緣圖功能。
兩個 Plan 同時啟用時，每個 Pass 會自動套用對應的邊緣引導係數：

```
Pass 1 EdgeWeight=0.0  → 無邊緣引導，大圖元自由鋪色
Pass 2 EdgeWeight=2.0  → 輕度邊緣引導，開始強化輪廓
Pass 3 EdgeWeight=4.0  → 強邊緣引導，細節精準對齊邊界
```

---

## B.9 驗證步驟

### 基本功能驗證
```bash
# 1. 跑原始模式（對照組）
./forza-painter --image test.png --shapes 200 --output baseline.json

# 2. 跑 MultiScale 模式
./forza-painter --image test.png --shapes 200 --multiscale \
    --save-pass-previews --output multiscale.json

# 3. 比對兩個 JSON 的橢圓分佈
# Pass 1 的橢圓應明顯大於 Pass 3
# 觀察 pass_1_preview.png vs pass_2_preview.png vs pass_3_preview.png
```

### 品質指標驗證
```python
# quality_check.py
from skimage.metrics import structural_similarity as ssim
from skimage.metrics import peak_signal_noise_ratio as psnr
import cv2

target = cv2.imread("test.png")
baseline_render = cv2.imread("baseline_render.png")
multiscale_render = cv2.imread("multiscale_render.png")

print(f"Baseline SSIM: {ssim(target, baseline_render, multichannel=True):.4f}")
print(f"MultiScale SSIM: {ssim(target, multiscale_render, multichannel=True):.4f}")
print(f"Baseline PSNR: {psnr(target, baseline_render):.2f} dB")
print(f"MultiScale PSNR: {psnr(target, multiscale_render):.2f} dB")
```

### 預期輸出範例
```
Baseline SSIM:    0.7412
MultiScale SSIM:  0.8651   ← 目標提升 ~15%
Baseline PSNR:    24.3 dB
MultiScale PSNR:  27.8 dB  ← 目標提升 ~3.5 dB
```

---

## 附錄：常見問題處理

### Q1：縮放後座標轉換不準確
確認 `rescaleShapeCoords` 使用的是 `float64` 乘法後再取整，不要用整數除法。

### Q2：Pass 之間畫布銜接有色差
Pass 2 和 Pass 3 的起始畫布是從 Pass 1 的輸出放大而來，BiLinear 插值會有輕微模糊。
這是預期行為，後續 Pass 的小圖元會自動補齊細節。

### Q3：MultiScale 模式比原始模式慢
圖像縮放本身開銷極小（< 1ms）。若速度下降，檢查：
- Pass 1 是否意外用了原始解析度（ScaleSize 應為 64）
- OpenCL buffer 是否在每個 Pass 都重新分配（應在 Pass 開始時重用）

### Q4：Pass 3 橢圓太小看不出來
調高 Pass 3 的 `MinRadius`，或增加 `ShapeCount` 比例：
```go
// 更激進的高頻細節配置
p3Config.MinRadius = 0.008  // 稍大一點的最小半徑
p3Config.ShapeCount = int(float64(totalShapes) * 0.55) // 增加到 55%
```

---

*計畫書版本：v1.0 | 目標系統：Geometrize Go + OpenCL | 預期品質提升：SSIM +15~20%*
