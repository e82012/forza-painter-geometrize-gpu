# Forza Painter Geometrize GPU Version

**Forza is a trademark of Microsoft. This project is unofficial and not affiliated with or endorsed by Microsoft.**

這是基於 [forza-painter](https://github.com/forza-painter/forza-painter) 的第三方 Geometrize 幾何形狀（JSON）生成工具。透過 GPU 加速與現代演算法改良，大幅提升圖案擬合品質與每秒運算效率。

* [原始簡體中文 README 說明 (備份)](README.original.md)
* [English README (Original)](README.en.md)

---

## 🚀 自 `main` 分支出發後的優化與改良項目

本分支 `feature/edge-guided-sampling` 主要加入了以下幾項調整與優化項目：

### 1. CMA-ES 最佳化演算法 (Covariance Matrix Adaptation Evolution Strategy)
* **原理說明**：使用協方差矩陣自適應進化策略（CMA-ES）來尋找橢圓的 5 維參數（中心位置 $X, Y$、長短軸半徑 $RX, RY$、旋轉角度 $\theta$）。
* **說明**：相較於原本的爬山演算法，提供另一種多維度參數搜尋的選擇，藉此調整幾何形狀的擬合細節。

### 2. 邊緣引導重要性取樣 (Edge-Guided Importance Sampling)
* **原理說明**：在引擎初始化時，透過 Sobel 濾波器計算目標畫作的邊緣強度，生成「邊緣強度地圖 (Edge Map)」。當 GPU 計算候選形狀的誤差分數（Pixel Error）時，若像素位於強烈邊緣，將被套用額外的誤差權重。
* **說明**：引導幾何橢圓偏向高細節區域（如線條、輪廓處），以改善邊緣部分的擬合度。

### 3. 多尺度階層式擬合 (Multi-Scale Hierarchical Fitting)
* **原理說明**：提供「由粗到細 (Coarse-to-Fine)」的漸進式擬合流程。
* **說明**：先在低解析度下擬合出畫作的大塊色彩與基礎幾何（背景），再逐步提高解析度並使用較小的橢圓進行高頻細節微調。

### 4. Web UI 瀏覽器控制台與實時日誌
* **說明**：提供基於 Node.js 的網頁控制介面。支援載入圖像、設定擬合參數、動態檢視 Canvas 預覽圖，並可從網頁端同步監看後端 Go 引擎的運行日誌與運算耗時。

### 5. Ring Buffer 非同步管線優化
* **說明**：底層採用雙緩衝區與 Ring Buffer 非同步管線機制（`ringSize = 3`），讓 CPU 在執行 CMA-ES 採樣的同時，GPU 能並行處理前一次的圖形套用（Apply）與誤差計算，以提升運算時的並行效率。


---

## 🛠️ 編譯與安裝

### 環境需求
* Go w/ CGO >= v1.24
* OpenCL-SDK >= v3.0.19

### 編譯 Windows 版本
1. 克隆本專案。
2. 下載 [OpenCL-SDK](https://github.com/KhronosGroup/OpenCL-SDK/releases/tag/v2025.07.23) 的 Windows 版本，並放置於 `/OpenCL-SDK` 目錄中。
3. 於 PowerShell 中執行編譯腳本：
   ```powershell
   powershell -ExecutionPolicy Bypass -File "build-opencl.ps1"
   ```

---

## 💻 開始使用

### 命令行參數說明

```bash
Usage: forza-painter-geometrize-go.exe [--settings path.ini|--profile name] [--output path] [--preview path] [--seed n] [--edge-weight w] [--multiscale] [--save-pass-previews] <image-path>
```

| 參數 | 說明 | 預設值 |
| :--- | :--- | :--- |
| `--settings` | 指定設定檔 `.ini` 路徑 | 空 |
| `--profile` | 指定 `./settings` 底下的 Profile 名稱片斷 | 空 |
| `--output` | 輸出幾何 JSON 檔案的路徑前綴 | 輸入圖片路徑 |
| `--preview` | 輸出的即時預覽圖 PNG 路徑 | 空 |
| `--seed` | 隨機數種子，用以重現生成結果 | `0` |
| `--edge-weight` | 邊緣引導重要性取樣權重（`0` 為停用，建議設為 `2.0` ~ `5.0`） | `-1.0` (停用) |
| `--multiscale` | 啟用多尺度階層式擬合（Coarse-to-Fine） | `false` (關閉) |
| `--save-pass-previews` | 在多尺度階層式擬合的每個 pass 結束後保存預覽圖 | `false` (關閉) |

### 命令行執行範例

* **以 OpenCL 加速執行並啟用邊緣引導擬合與多尺度階層式擬合**：
  ```cmd
  forza-painter-geometrize-go.exe C:\work\forza\test.png --settings "C:\work\forza\settings\c.ini" --preview "C:\work\forza\preview.png" --edge-weight 3.0 --multiscale
  ```

---

## 🌐 Web UI 使用說明

本分支提供基於網頁的控制台，方便在瀏覽器中操作並即時監看日誌與 Canvas 進度。

### 啟動步驟
1. 確保已將編譯後的 Go 執行檔命名為 `forza-painter-geometrize-go-v1.0.exe` 並放置於專案根目錄下（若名稱不同，可修改 `server.js` 中的 `execPath` 配置）。
2. 雙擊執行 `start_server.bat`，或在終端機手動執行：
   ```bash
   node server.js
   ```
3. 服務啟動後，將會自動在預設瀏覽器中開啟 `http://localhost:8080`（若未自動開啟請手動於瀏覽器輸入該網址）。
4. 在網頁中上傳圖片，選擇預設品質（低、中、高）或自訂擬合參數後，點擊「開始」即可實時檢視預覽 Canvas，並在下方的日誌主控台觀察 GPU 引擎內部的每一輪最佳化與運算耗時。