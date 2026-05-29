# Forza Painter Geometrize GPU Version

**Forza is a trademark of Microsoft. This project is unofficial and not affiliated with or endorsed by Microsoft.**

這是基於 [forza-painter](https://github.com/forza-painter/forza-painter) 的第三方 Geometrize 幾何形狀（JSON）生成工具。透過 GPU 加速與現代演算法改良，大幅提升圖案擬合品質與每秒運算效率。

* [原始簡體中文 README 說明 (備份)](README.original.md)
* [English README (Original)](README.en.md)

---

## 🚀 自 `main` 分支出發後的優化與改良項目

本分支 `feature/edge-guided-sampling-v3` 在 v2 特性基礎上，進一步整合 `main` 分支的遊戲相容性修復：

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
* **說明**：提供基於 Node.js 的網頁控制介面。支援載入圖像、設定擬合參數、動態檢視 Canvas 預覽圖，並可從網頁端同步監看後端 Go 引擎的運行日誌與運算耗時。**在 v3 中，我們補齊了多尺度擬合模式 (Multi-Scale) 下的預覽圖保存與進度日誌輸出機制，解決了先前 Web UI 在多尺度模式下無法顯示即時預覽圖、張數與剩餘時間的問題。**

### 5. Ring Buffer 非同步管線優化
* **說明**：底層採用雙緩衝區與 Ring Buffer 非同步管線機制（`ringSize = 3`），讓 CPU 在執行 CMA-ES 採樣的同時，GPU 能並行處理前一次的圖形套用（Apply）與誤差計算，以提升運算時的並行效率。

### 6. 車貼記憶體注入器 (Livery Memory Injector)
* **說明**：新增獨立的記憶體匯入分頁。支援從本地歷史幾何 JSON 中進行選取，或拖曳上傳外部 JSON。透過 Node.js 後端安全調用 Windows VRAM 記憶體寫入核心，將幾何形狀資料熱注入 (Inject)至遊戲當前的車貼圖層表中，實現無須在遊戲內手動繪製，便能一鍵熱渲染出精美幾何圖案。

### 7. 無損尺度校準與浮點數高精度匯出 (Lossless Scale Snapping & Float Export)
* **說明**：Forza 遊戲在匯入幾何時，對橢圓縮放比例（radius/63）進行小數點後兩位截斷。v3 整合了 `main` 的前置「無損對齊 (Snapping)」機制（`snapToValidRX`），讓所有生成的形狀天生符合截斷精度，大幅提升 GPU 搜尋效率。同時 JSON 數據升級為 `[]float64` 浮點數儲存，保證本地預覽與遊戲最終匯入效果完全無損一致 (Lossless Roundtrip)。

### 8. 斷點續傳 (Checkpoint Resume)
* **說明**：支援從先前已儲存的幾何 JSON 檔案中斷點恢復，繼續進行後續的形狀擬合。可透過 `--resume` CLI 參數或設定檔中的 `loadGeometry` 選項指定檢查點路徑。

### 9. 均勻 RGB 誤差評估 (Uniform RGB MSE) — 品質優化回退
* **說明**：為了解決感知色彩空間權重 (Weighted RGB MSE) 在高對比或特定色彩邊緣產生的「細節模糊化」缺陷，**v3 已將 Go 端與 OpenCL Kernel 的運算核心完整還原為傳統均勻線性 RGB MSE**。這確保了每個色彩通道在優化時獲得同等重視，完美還原文字輪廓與銳利邊緣。

### 10. 全域高強度邊緣引導 (Global Edge Weight Constancy)
* **說明**：我們移除了先前實作的「自適應邊緣引導權重衰減」機制。在擬合流程的全階段中，**邊緣引導權重維持全域 100% 高強度作用**，以避免大尺寸背景橢圓在初期定位時偏離邊緣界線，從根本上解決了底色邊界模糊化的缺陷，顯著提升整體圖形貼合度。

### 11. CMA-ES 疊代收斂早停機制 (CMA-ES Early Termination)
* **原理說明**：在 CMA-ES 進化迴圈中，實時追蹤分數改善量。當連續 8 代沒有產生顯著的分數改進（閾值 $10^{-8}$）時，即自動提早結束該輪進化。
* **說明**：避免 GPU 在已經收斂的形狀上浪費運算資源，顯著提升執行效率與回應速度。

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

## 🌐 Web UI 使用說明

本分支提供基於網頁的控制台，方便在瀏覽器中操作並即時監看日誌與 Canvas 進度。

### 啟動步驟
1. 確保已將編譯後的 Go 執行檔命名為 `forza-painter-geometrize-go.exe` 並放置於專案根目錄下（若名稱不同，可修改 `server.js` 中的 `execPath` 配置）。
2. 雙擊執行 `start_server.bat`，或在終端機手動執行：
   ```bash
   node server.js
   ```
3. 服務啟動後，將會自動在預設瀏覽器中開啟 `http://localhost:8080`（若未自動開啟請手動於瀏覽器輸入該網址）。
4. 在網頁中上傳圖片，選擇預設品質（低、中、高）或自訂擬合參數後，點擊「開始」即可實時檢視預覽 Canvas，並在下方的日誌主控台觀察 GPU 引擎內部的每一輪最佳化與運算耗時。

---

### 🚗 幾何車貼記憶體匯入說明

記憶體注入功能（將擬合好的 JSON 數據熱寫入遊戲）可點擊 Web UI 頂端的「記憶體匯入遊戲」頁籤進行操作。

#### 先決條件
1. **Python 3 環境**：請確保本機已安裝 Python 3 且已將其加入環境變數（Windows 系統建議可於 Terminal 輸入 `python --version` 驗證）。
2. **系統管理員權限**：**必須以系統管理員權限執行** `start_server.bat`（或在管理員權限 CMD/PowerShell 視窗中啟動 `node server.js`），否則後端將因權限不足（Access Denied）無法開啟並寫入遊戲的記憶體。
3. **遊戲編輯狀態**：啟動《極限競速》遊戲，前往「應用貼紙與形狀」設計編輯頁面，建立一個包含足夠圖層的空白群組並保持在編輯模式。

#### 使用步驟
1. 在 Web UI 導覽切換至 **「記憶體匯入遊戲」**。
2. 選擇幾何來源：
   * **歷史 JSON 選擇**：系統會自動列出目前 `img_json/` 目錄下所有先前擬合生成的幾何 JSON 檔案。
   * **外部 JSON 上傳**：可直接拖放或點選上傳其他地方生成的符合 Geometrize 格式的 JSON 檔案。
3. 點擊 **「開始寫入遊戲記憶體 (Inject)」** 啟動作業，即可在右側的動態終端日誌控制台監測基底地址掃描與寫入進度。
4. 寫入完成後，切換回遊戲編輯介面即可看到生成好的圖案。如需中止，可點擊「中止寫入」安全終止背景子進程。

### 命令行參數說明

```bash
Usage: forza-painter-geometrize-go.exe [--settings path.ini|--profile name] [--output path] [--preview path] [--seed n] [--edge-weight w] [--multiscale] [--save-pass-previews] [--resume checkpoint.json] <image-path>
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
| `--resume` | 從指定的幾何 JSON 斷點檔案繼續擬合 | 空 |

### 命令行執行範例

* **以 OpenCL 加速執行並啟用邊緣引導擬合與多尺度階層式擬合**：
  ```cmd
  forza-painter-geometrize-go.exe C:\work\forza\test.png --settings "C:\work\forza\settings\c.ini" --preview "C:\work\forza\preview.png" --edge-weight 3.0 --multiscale
  ```
* **從已存的 1500 形狀 checkpoint 繼續擬合至 3000 形狀**：
  ```cmd
  forza-painter-geometrize-go.exe C:\work\forza\test.png --settings "C:\work\forza\settings\c.ini" --resume "C:\work\forza\test_1500.json" --preview "C:\work\forza\preview.png"
  ```

---