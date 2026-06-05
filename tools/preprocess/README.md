# AI 前處理風格化管線 (Preprocess Pipeline)

## 一、計畫背景與可行性結論

最初的構想是使用 `SAM2 + OpenCV + VTracer` 來取代現行 v3 引擎的橢圓搜索。
經過可行性分析，發現 VTracer 輸出的 `SVG Bézier Path` 無法被 Forza 遊戲的 `旋轉橢圓` 完美相容。Bézier 曲線轉橢圓是一個有損且複雜的逆向工程，會失去 VTracer 帶來的優勢。

因此，本管線採用**混合策略 (Hybrid Pipeline)**：
- **保留 v3 Geometrize 引擎**：負責最終的高精細度橢圓擬合與輸出。
- **新增 Python 前處理管線**：在圖片進入 v3 引擎前，透過 OpenCV (未來可加入 Real-ESRGAN/AnimeGAN) 對輸入圖像進行「風格化前處理」，使其變成「橢圓友善」的平坦色塊與銳利邊緣，大幅降低 Geometrize 擬合的難度，並提升邊緣清晰度。

## 二、目前進度：Phase 1 (純 OpenCV 實作)

本目錄 (`tools/preprocess`) 目前實作了 Phase 1 的處理管線，無需深度學習框架，可直接透過 Python 執行。

### 功能模組
1. **自動分類器 (`classifier.py`)**：自動分析圖片特徵，決定要使用 Portrait, Logo, Illustration 或 Landscape 哪一種處理策略。
2. **雙邊濾波與卡通化 (`style.py`)**：負責抹平雜訊與微小紋理，保留主體輪廓。
3. **色彩量化 (`quantize.py`)**：K-Means (目前在 Logo 與 Portrait 中預設關閉以避免色帶效應)。
4. **邊緣強化 (`edge.py`)**：使用 Unsharp Mask，讓平坦色塊的邊緣變得極度銳利，引導 v3 的 Importance Map 精準對齊。

### 使用方式
```powershell
# 1. 安裝依賴
pip install -r requirements.txt

# 2. 執行前處理
python run.py --input "你的圖片路徑.png"

# 將產出的 _preprocessed.png 餵給原本的 v3 引擎進行測試
```

## 三、未來計畫
- **Phase 2**：加入 `Real-ESRGAN` 超解析放大，為人像與 Logo 提供更多像素細節。
- **Phase 3**：整合進 `server.js`，讓 Web UI 在呼叫 v3 前自動先跑前處理。
