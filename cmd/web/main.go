package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	MaxPreviewSize int    `json:"maxPreviewSize"`
	MaxResolution  int    `json:"maxResolution"`
	MutatedSamples int    `json:"mutatedSamples"`
	RandomSamples  int    `json:"randomSamples"`
	StopAt         int    `json:"stopAt"`
	SaveAt         string `json:"saveAt"`
}

type StartRequest struct {
	Filename       string `json:"filename"`
	Quality        string `json:"quality"`
	CustomSettings Config `json:"customSettings"`
}

type JobStatus struct {
	Status         string    `json:"status"` // idle, running, completed, terminated, failed
	CurrentStep    int       `json:"currentStep"`
	TotalSteps     int       `json:"totalSteps"`
	StepTimeMs     int       `json:"stepTimeMs"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	ElapsedTimeS   float64   `json:"elapsedTimeS"`
	RemainingTimeS float64   `json:"remainingTimeS"`
	ErrorMsg       string    `json:"errorMsg"`
}

var (
	statusMutex         sync.Mutex
	currentJob          *exec.Cmd
	jobStatus           = JobStatus{Status: "idle"}
	stopChan            chan struct{}
	lastPreviewBuffer   []byte // 記憶體預覽快取
	activeInputFilename string // 原始上傳圖片名稱
)

func main() {
	// 靜態檔案路由
	http.Handle("/", http.FileServer(http.Dir("./cmd/web/static")))

	// API 路由
	http.HandleFunc("/api/upload", handleUpload)
	http.HandleFunc("/api/start", handleStart)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/stop", handleStop)
	http.HandleFunc("/api/preview-image", handlePreviewImage)

	port := ":8080"
	fmt.Printf("Geometrize Web Server 正在執行於 http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

// 處理圖片上傳
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "僅接受 POST 請求", http.StatusMethodNotAllowed)
		return
	}

	// 限制上傳檔案大小為 20MB
	r.ParseMultipartForm(20 << 20)

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "讀取檔案失敗: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 建立 img_pre 目錄（如果不存在）
	os.MkdirAll("./img_pre", os.ModePerm)

	// 生成安全的檔案名稱（保留副檔名）
	ext := filepath.Ext(handler.Filename)
	base := strings.TrimSuffix(handler.Filename, ext)
	filename := fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext)
	targetPath := filepath.Join("./img_pre", filename)

	dst, err := os.Create(targetPath)
	if err != nil {
		http.Error(w, "無法建立目標檔案: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "儲存檔案失敗: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "success",
		"filename": filename,
	})
}

// 處理開始生成
func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "僅接受 POST 請求", http.StatusMethodNotAllowed)
		return
	}

	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "無效的 JSON 請求", http.StatusBadRequest)
		return
	}

	statusMutex.Lock()
	if jobStatus.Status == "running" {
		statusMutex.Unlock()
		http.Error(w, "已有任務正在運行中", http.StatusConflict)
		return
	}

	// 初始化狀態與重置快取
	lastPreviewBuffer = nil
	activeInputFilename = req.Filename
	jobStatus = JobStatus{
		Status:    "running",
		StartTime: time.Now(),
	}
	stopChan = make(chan struct{})
	statusMutex.Unlock()

	// 根據品質等級決定參數：統一最大圖層為 1200 層
	var conf Config
	switch req.Quality {
	case "low":
		conf = Config{
			MaxPreviewSize: 500,
			MaxResolution:  1200,
			MutatedSamples: 800,
			RandomSamples:  8000,
			StopAt:         1200,
			SaveAt:         "1200",
		}
	case "medium":
		conf = Config{
			MaxPreviewSize: 500,
			MaxResolution:  1200,
			MutatedSamples: 1500,
			RandomSamples:  25000,
			StopAt:         1200,
			SaveAt:         "600,1200",
		}
	case "high":
		conf = Config{
			MaxPreviewSize: 500,
			MaxResolution:  1200,
			MutatedSamples: 3000,
			RandomSamples:  60000,
			StopAt:         1200,
			SaveAt:         "300,600,900,1200",
		}
	case "custom":
		conf = req.CustomSettings
		// 確保最大解析度統一或以參數為準，限制最大不超過 2048 避免崩潰
		if conf.MaxResolution == 0 {
			conf.MaxResolution = 1200
		}
		if conf.StopAt == 0 {
			conf.StopAt = 1000
		}
		if conf.MutatedSamples == 0 {
			conf.MutatedSamples = 2000
		}
		if conf.RandomSamples == 0 {
			conf.RandomSamples = 30000
		}
		// 生成 saveAt 序列
		saveAtSlice := []string{}
		for i := 500; i <= conf.StopAt; i += 500 {
			saveAtSlice = append(saveAtSlice, strconv.Itoa(i))
		}
		if len(saveAtSlice) == 0 || conf.StopAt % 500 != 0 {
			saveAtSlice = append(saveAtSlice, strconv.Itoa(conf.StopAt))
		}
		conf.SaveAt = strings.Join(saveAtSlice, ",")
	default:
		conf = Config{
			MaxPreviewSize: 500,
			MaxResolution:  1200,
			MutatedSamples: 2000,
			RandomSamples:  30000,
			StopAt:         1500,
			SaveAt:         "500,1000,1500",
		}
	}

	// 寫入臨時設定檔
	os.MkdirAll("./settings", os.ModePerm)
	tempIniPath := filepath.Join("settings", "temp_web.ini")
	iniContent := fmt.Sprintf(
		"description = Web Temp Configuration\nmaxPreviewSize = %d\nmaxResolution = %d\nmaxThreads = 0\nmutatedSamples = %d\nposterizeLevels = 20\npreviewEvery = 20\nrandomSamples = %d\nsaveAt = %s\nsaveEvery = 50\nstopAt = %d\n",
		conf.MaxPreviewSize, conf.MaxResolution, conf.MutatedSamples, conf.RandomSamples, conf.SaveAt, conf.StopAt,
	)

	if err := os.WriteFile(tempIniPath, []byte(iniContent), 0644); err != nil {
		statusMutex.Lock()
		jobStatus.Status = "failed"
		jobStatus.ErrorMsg = "無法寫入設定檔: " + err.Error()
		statusMutex.Unlock()
		http.Error(w, jobStatus.ErrorMsg, http.StatusInternalServerError)
		return
	}

	// 確保目錄存在且刪除舊的預覽圖片
	os.MkdirAll("./img_preview", os.ModePerm)
	os.MkdirAll("./img_json", os.ModePerm)
	os.Remove(filepath.Join("img_preview", "web_preview.png"))

	// 非同步啟動子進程
	go runGeometrize(req.Filename, tempIniPath, conf.StopAt)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// 執行二進位執行檔的背景程序
func runGeometrize(filename, iniPath string, stopAt int) {
	inputPath := filepath.Join("img_pre", filename)
	previewPath := filepath.Join("img_preview", "web_preview.png")
	
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	outputPath := filepath.Join("img_json", base)

	// 獲取執行檔絕對路徑
	execPath, err := filepath.Abs("forza-painter-geometrize-go-v1.0.exe")
	if err != nil {
		updateJobError("尋找執行檔失敗: " + err.Error())
		return
	}

	cmd := exec.Command(execPath, inputPath, "-preview", previewPath, "-settings", iniPath, "-output", outputPath)
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		updateJobError("管道建立失敗: " + err.Error())
		return
	}
	cmd.Stderr = cmd.Stdout // 合併標準錯誤到標準輸出

	statusMutex.Lock()
	currentJob = cmd
	jobStatus.TotalSteps = stopAt
	statusMutex.Unlock()

	if err := cmd.Start(); err != nil {
		updateJobError("啟動程式失敗: " + err.Error())
		return
	}

	// 讀取並解析進度
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// 範例：[12/100] Step completed in 185ms
		if strings.Contains(line, "Step completed in") {
			parseProgress(line)
		}
	}

	err = cmd.Wait()

	statusMutex.Lock()
	defer statusMutex.Unlock()

	select {
	case <-stopChan:
		jobStatus.Status = "terminated"
		jobStatus.EndTime = time.Now()
	default:
		if err != nil {
			jobStatus.Status = "failed"
			jobStatus.ErrorMsg = "程式異常終止: " + err.Error()
			jobStatus.EndTime = time.Now()
		} else {
			jobStatus.Status = "completed"
			jobStatus.CurrentStep = jobStatus.TotalSteps
			jobStatus.RemainingTimeS = 0
			jobStatus.EndTime = time.Now()
		}
	}

	// 任務結束時，將最終的預覽圖讀入記憶體快取
	if bytes, readErr := os.ReadFile(previewPath); readErr == nil {
		lastPreviewBuffer = bytes
	}

	// 自動清理 img_preview 目錄下的所有實體暫存檔案
	if files, scanErr := os.ReadDir("img_preview"); scanErr == nil {
		for _, file := range files {
			os.Remove(filepath.Join("img_preview", file.Name()))
		}
	}

	currentJob = nil
}

func parseProgress(line string) {
	// [12/100] Step completed in 185ms
	statusMutex.Lock()
	defer statusMutex.Unlock()

	bracketOpen := strings.Index(line, "[")
	slash := strings.Index(line, "/")
	bracketClose := strings.Index(line, "]")
	completedIn := strings.Index(line, "completed in ")
	ms := strings.Index(line, "ms")

	if bracketOpen != -1 && slash != -1 && bracketClose != -1 && slash > bracketOpen && bracketClose > slash {
		currStr := line[bracketOpen+1 : slash]
		totStr := line[slash+1 : bracketClose]

		curr, err1 := strconv.Atoi(currStr)
		tot, err2 := strconv.Atoi(totStr)

		if err1 == nil && err2 == nil {
			jobStatus.CurrentStep = curr
			jobStatus.TotalSteps = tot
		}
	}

	if completedIn != -1 && ms != -1 && ms > completedIn {
		timeStr := line[completedIn+len("completed in ") : ms]
		timeVal, err := strconv.Atoi(timeStr)
		if err == nil {
			jobStatus.StepTimeMs = timeVal
		}
	}

	// 計算運行時間與剩餘時間
	jobStatus.ElapsedTimeS = time.Since(jobStatus.StartTime).Seconds()
	remainingSteps := jobStatus.TotalSteps - jobStatus.CurrentStep
	if remainingSteps > 0 && jobStatus.StepTimeMs > 0 {
		// 剩餘時間 = 剩餘步數 * 平均單步時間(秒)
		jobStatus.RemainingTimeS = float64(remainingSteps) * (float64(jobStatus.StepTimeMs) / 1000.0)
	} else {
		jobStatus.RemainingTimeS = 0
	}
}

func updateJobError(msg string) {
	statusMutex.Lock()
	jobStatus.Status = "failed"
	jobStatus.ErrorMsg = msg
	jobStatus.EndTime = time.Now()
	statusMutex.Unlock()
}

// 處理狀態查詢
func handleStatus(w http.ResponseWriter, r *http.Request) {
	statusMutex.Lock()
	defer statusMutex.Unlock()

	if jobStatus.Status == "running" {
		jobStatus.ElapsedTimeS = time.Since(jobStatus.StartTime).Seconds()
		remainingSteps := jobStatus.TotalSteps - jobStatus.CurrentStep
		if remainingSteps > 0 && jobStatus.StepTimeMs > 0 {
			jobStatus.RemainingTimeS = float64(remainingSteps) * (float64(jobStatus.StepTimeMs) / 1000.0)
		}
	} else if jobStatus.Status == "completed" || jobStatus.Status == "terminated" || jobStatus.Status == "failed" {
		if !jobStatus.StartTime.IsZero() && !jobStatus.EndTime.IsZero() {
			jobStatus.ElapsedTimeS = jobStatus.EndTime.Sub(jobStatus.StartTime).Seconds()
		}
		jobStatus.RemainingTimeS = 0
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	json.NewEncoder(w).Encode(jobStatus)
}

// 處理中止任務
func handleStop(w http.ResponseWriter, r *http.Request) {
	statusMutex.Lock()
	defer statusMutex.Unlock()

	if jobStatus.Status != "running" || currentJob == nil {
		http.Error(w, "目前沒有正在運行的任務", http.StatusBadRequest)
		return
	}

	close(stopChan) // 通知背景 goroutine 是被終止的
	
	// 在 Windows 上強制 Kill 子進程
	if err := currentJob.Process.Kill(); err != nil {
		http.Error(w, "無法終止進程: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

// 處理預覽圖片請求（防讀取寫入衝突的防禦性讀取）
func handlePreviewImage(w http.ResponseWriter, r *http.Request) {
	statusMutex.Lock()
	defer statusMutex.Unlock()

	// 1. 優先使用最終緩存
	if lastPreviewBuffer != nil {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Write(lastPreviewBuffer)
		return
	}

	previewPath := filepath.Join("img_preview", "web_preview.png")

	// 2. 嘗試防禦性讀取 web_preview.png，進行最多 3 次 retry
	var fileBytes []byte
	var err error
	for i := 0; i < 3; i++ {
		fileBytes, err = os.ReadFile(previewPath)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 3. 讀取失敗時，尋找已釋放鎖定的最新歷史步數預覽圖
	if err != nil {
		if files, scanErr := os.ReadDir("img_preview"); scanErr == nil {
			var maxStep int = -1
			var targetFile string = ""
			for _, f := range files {
				name := f.Name()
				if strings.HasPrefix(name, "web_preview.") && strings.HasSuffix(name, ".png") && name != "web_preview.png" {
					parts := strings.Split(name, ".")
					if len(parts) >= 3 {
						if stepNum, convErr := strconv.Atoi(parts[1]); convErr == nil && stepNum > maxStep {
							maxStep = stepNum
							targetFile = name
						}
					}
				}
			}

			if targetFile != "" {
				if fallbackBytes, readErr := os.ReadFile(filepath.Join("img_preview", targetFile)); readErr == nil {
					w.Header().Set("Content-Type", "image/png")
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Write(fallbackBytes)
					return
				}
			}
		}

		// 4. 若連歷史預覽圖都沒有，防禦性回傳原圖以防 404 破圖
		if activeInputFilename != "" {
			if originalBytes, readErr := os.ReadFile(filepath.Join("img_pre", activeInputFilename)); readErr == nil {
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Write(originalBytes)
				return
			}
		}

		http.Error(w, "預覽圖片尚未生成或無法讀取", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(fileBytes)
}
