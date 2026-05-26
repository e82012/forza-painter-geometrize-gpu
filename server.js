const http = require('http');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');
const readline = require('readline');

const PORT = 8080;
const STATIC_DIR = path.join(__dirname, 'cmd', 'web', 'static');

let currentJob = null;
let stopChanTriggered = false;
let lastPreviewBuffer = null; // 用於緩存最終的預覽圖片 Buffer
let activeInputFilename = ''; // 用於記錄當前運行的輸入原圖名稱
let jobStatus = {
  status: 'idle',
  currentStep: 0,
  totalSteps: 0,
  stepTimeMs: 0,
  startTime: null,
  endTime: null,
  elapsedTimeS: 0,
  remainingTimeS: 0,
  errorMsg: '',
  logs: []
};

// 守護函式：確保 Node 進程關閉或崩潰時，也一定會將背景運行的 Go/GPU 子進程終止，防止資源洩漏
function cleanupChildProcess() {
  if (currentJob) {
    console.log('檢測到 Web 伺服器即將退出，正在強制終止背景幾何擬合進程...');
    try {
      currentJob.kill('SIGKILL');
    } catch (e) {
      // 忽略錯誤
    }
  }
}

// 監聽進程訊號與退出事件
process.on('exit', cleanupChildProcess);
process.on('SIGINT', () => {
  cleanupChildProcess();
  process.exit(0);
});
process.on('SIGTERM', () => {
  cleanupChildProcess();
  process.exit(0);
});
process.on('uncaughtException', (err) => {
  console.error('Web 伺服器發生未捕獲的異常，正在緊急退出並清理資源:', err);
  cleanupChildProcess();
  process.exit(1);
});

// 輔助函式：確保目錄存在
function ensureDirExists(dirPath) {
  if (!fs.existsSync(dirPath)) {
    fs.mkdirSync(dirPath, { recursive: true });
  }
}

// 建立需要的資料夾
ensureDirExists(path.join(__dirname, 'img_pre'));
ensureDirExists(path.join(__dirname, 'img_preview'));
ensureDirExists(path.join(__dirname, 'img_json'));
ensureDirExists(path.join(__dirname, 'settings'));

const server = http.createServer((req, res) => {
  const parsedUrl = new URL(req.url, `http://${req.headers.host}`);
  const pathname = parsedUrl.pathname;

  // ----------------------------------------------------
  // 靜態檔案路由
  // ----------------------------------------------------
  if (req.method === 'GET' && (pathname === '/' || pathname === '/index.html' || pathname === '/app.css' || pathname === '/app.js')) {
    const filename = pathname === '/' ? 'index.html' : pathname.slice(1);
    const filePath = path.join(STATIC_DIR, filename);
    
    let contentType = 'text/html';
    if (filename.endsWith('.css')) contentType = 'text/css';
    if (filename.endsWith('.js')) contentType = 'application/javascript';

    fs.readFile(filePath, (err, data) => {
      if (err) {
        res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
        res.end('檔案不存在');
      } else {
        res.writeHead(200, { 'Content-Type': contentType });
        res.end(data);
      }
    });
    return;
  }

  // ----------------------------------------------------
  // 預覽圖片讀取 API (防禦性讀取，處理鎖定)
  // ----------------------------------------------------
  if (req.method === 'GET' && pathname === '/api/preview-image') {
    // 優先回傳記憶體中的緩存圖片，以防止實體檔案已遭清理
    if (lastPreviewBuffer) {
      res.writeHead(200, { 
        'Content-Type': 'image/png',
        'Cache-Control': 'no-cache, no-store, must-revalidate'
      });
      res.end(lastPreviewBuffer);
      return;
    }

    const previewPath = path.join(__dirname, 'img_preview', 'web_preview.png');
    
    // 防禦性讀取：進行最多 3 次 retry，每次間隔 50ms
    let attempts = 0;
    const readPreview = () => {
      fs.readFile(previewPath, (err, data) => {
        if (err) {
          // 由於 Windows 下 web_preview.png 可能被 Go 進程鎖定寫入，讀取失敗時，非同步尋找已釋放鎖定的最新歷史步數圖片
          const previewDir = path.join(__dirname, 'img_preview');

          fs.readdir(previewDir, (scanErr, files) => {
            if (scanErr || !files || files.length === 0) {
              fallbackToOriginal();
              return;
            }

            const previewFiles = files.filter(f => f.startsWith('web_preview.') && f.endsWith('.png') && f !== 'web_preview.png');
            
            if (previewFiles.length > 0) {
              // 排序並尋找最高步數的歷史檔案
              let maxStep = -1;
              let targetFile = '';
              
              for (const file of previewFiles) {
                const parts = file.split('.');
                if (parts.length >= 3) {
                  const stepNum = parseInt(parts[1], 10);
                  if (!isNaN(stepNum) && stepNum > maxStep) {
                    maxStep = stepNum;
                    targetFile = file;
                  }
                }
              }

              if (targetFile) {
                const fallbackPath = path.join(previewDir, targetFile);
                fs.readFile(fallbackPath, (readErr, fallbackData) => {
                  if (readErr) {
                    fallbackToOriginal();
                  } else {
                    res.writeHead(200, { 
                      'Content-Type': 'image/png',
                      'Cache-Control': 'no-cache, no-store, must-revalidate'
                    });
                    res.end(fallbackData);
                  }
                });
              } else {
                fallbackToOriginal();
              }
            } else {
              fallbackToOriginal();
            }
          });

          // 內部輔助非同步 Fallback 函數
          function fallbackToOriginal() {
            if (activeInputFilename) {
              const originalPath = path.join(__dirname, 'img_pre', activeInputFilename);
              fs.readFile(originalPath, (oriErr, oriData) => {
                if (oriErr) {
                  retryOr404();
                } else {
                  res.writeHead(200, { 
                    'Content-Type': 'image/png',
                    'Cache-Control': 'no-cache, no-store, must-revalidate'
                  });
                  res.end(oriData);
                }
              });
            } else {
              retryOr404();
            }
          }

          function retryOr404() {
            if (attempts < 3) {
              attempts++;
              setTimeout(readPreview, 50);
            } else {
              res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
              res.end('預覽圖片尚未生成或無法讀取');
            }
          }
        } else {
          res.writeHead(200, { 
            'Content-Type': 'image/png',
            'Cache-Control': 'no-cache, no-store, must-revalidate'
          });
          res.end(data);
        }
      });
    };
    readPreview();
    return;
  }

  // ----------------------------------------------------
  // 圖片上傳 API (流式接收，免依賴)
  // ----------------------------------------------------
  if (req.method === 'POST' && pathname === '/api/upload') {
    const rawFilename = req.headers['x-filename'] || 'upload.png';
    const filename = decodeURIComponent(rawFilename);
    const ext = path.extname(filename);
    const base = path.basename(filename, ext);
    
    // 生成安全名稱
    const safeName = `${base}_${Date.now()}${ext}`;
    const targetPath = path.join(__dirname, 'img_pre', safeName);

    const writeStream = fs.createWriteStream(targetPath);
    req.pipe(writeStream);

    req.on('error', (err) => {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'error', message: err.message }));
    });

    writeStream.on('finish', () => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'success', filename: safeName }));
    });
    return;
  }

  // ----------------------------------------------------
  // 開始任務 API
  // ----------------------------------------------------
  if (req.method === 'POST' && pathname === '/api/start') {
    let body = '';
    req.on('data', chunk => { body += chunk; });
    req.on('end', () => {
      try {
        const reqData = JSON.parse(body);
        
        if (jobStatus.status === 'running') {
          res.writeHead(409, { 'Content-Type': 'text/plain; charset=utf-8' });
          res.end('已有任務正在運行中');
          return;
        }

        // 初始化狀態
        stopChanTriggered = false;
        lastPreviewBuffer = null; // 重置舊緩存
        activeInputFilename = reqData.filename; // 記錄原圖檔名
        jobStatus = {
          status: 'running',
          currentStep: 0,
          totalSteps: 1200,
          stepTimeMs: 0,
          startTime: Date.now(),
          endTime: null,
          elapsedTimeS: 0,
          remainingTimeS: 0,
          errorMsg: '',
          logs: []
        };

        // 決定品質參數：統一預設最大圖層為 1200 層
        // 預設 (中等品質)
        let config = {
          maxPreviewSize: 500,
          maxResolution: 1200,
          mutatedSamples: 1500,
          randomSamples: 25000,
          stopAt: 1200,
          saveAt: '600,1200'
        };

        if (reqData.quality === 'low') {
          config = {
            maxPreviewSize: 500,
            maxResolution: 1200,
            mutatedSamples: 800,
            randomSamples: 8000,
            stopAt: 1200,
            saveAt: '1200'
          };
        } else if (reqData.quality === 'high') {
          config = {
            maxPreviewSize: 500,
            maxResolution: 1200,
            mutatedSamples: 3000,
            randomSamples: 60000,
            stopAt: 1200,
            saveAt: '300,600,900,1200'
          };
        } else if (reqData.quality === 'custom') {
          const custom = reqData.customSettings || {};
          config.maxResolution = custom.maxResolution || 1200;
          config.stopAt = custom.stopAt || 1000;
          config.mutatedSamples = custom.mutatedSamples || 2000;
          config.randomSamples = custom.randomSamples || 30000;
          config.maxPreviewSize = 500;

          // 生成 saveAt 序列
          const saveAtSlice = [];
          for (let i = 500; i <= config.stopAt; i += 500) {
            saveAtSlice.push(i);
          }
          if (saveAtSlice.length === 0 || config.stopAt % 500 !== 0) {
            saveAtSlice.push(config.stopAt);
          }
          config.saveAt = saveAtSlice.join(',');
        }

        jobStatus.totalSteps = config.stopAt;

        // 寫入臨時設定檔
        const tempIniPath = path.join(__dirname, 'settings', 'temp_web.ini');
        const iniContent = `description = Web Temp Configuration
maxPreviewSize = ${config.maxPreviewSize}
maxResolution = ${config.maxResolution}
maxThreads = 0
mutatedSamples = ${config.mutatedSamples}
posterizeLevels = 20
previewEvery = 20
randomSamples = ${config.randomSamples}
saveAt = ${config.saveAt}
saveEvery = 50
stopAt = ${config.stopAt}
`;
        fs.writeFileSync(tempIniPath, iniContent);

        // 刪除舊的預覽圖片
        const previewImgPath = path.join(__dirname, 'img_preview', 'web_preview.png');
        if (fs.existsSync(previewImgPath)) {
          fs.unlinkSync(previewImgPath);
        }

        // 啟動子進程
        const execPath = path.join(__dirname, 'forza-painter-geometrize-go-v1.0.exe');
        const ext = path.extname(reqData.filename);
        const base = path.basename(reqData.filename, ext);
        const args = [
          path.join('img_pre', reqData.filename),
          '-preview', path.join('img_preview', 'web_preview.png'),
          '-settings', path.join('settings', 'temp_web.ini'),
          '-output', path.join('img_json', base),
          '-multiscale',
          '-edge-weight', '3.0'
        ];

        currentJob = spawn(execPath, args, { cwd: __dirname });

        const appendLog = (line) => {
          if (!line) return;
          console.log(`[Engine] ${line}`);
          jobStatus.logs.push(line);
          if (jobStatus.logs.length > 200) {
            jobStatus.logs.shift();
          }
        };

        // 解析進度
        const rl = readline.createInterface({ input: currentJob.stdout });
        rl.on('line', (line) => {
          appendLog(line);
          if (line.includes('Step completed in')) {
            const bracketOpen = line.indexOf('[');
            const slash = line.indexOf('/');
            const bracketClose = line.indexOf(']');
            const completedIn = line.indexOf('completed in ');
            const ms = line.indexOf('ms');

            if (bracketOpen !== -1 && slash !== -1 && bracketClose !== -1 && slash > bracketOpen && bracketClose > slash) {
              const curr = parseInt(line.substring(bracketOpen + 1, slash), 10);
              const tot = parseInt(line.substring(slash + 1, bracketClose), 10);
              if (!isNaN(curr) && !isNaN(tot)) {
                jobStatus.currentStep = curr;
                jobStatus.totalSteps = tot;
              }
            }

            if (completedIn !== -1 && ms !== -1 && ms > completedIn) {
              const timeVal = parseInt(line.substring(completedIn + 'completed in '.length, ms), 10);
              if (!isNaN(timeVal)) {
                jobStatus.stepTimeMs = timeVal;
              }
            }

            // 更新運行時間
            jobStatus.elapsedTimeS = (Date.now() - jobStatus.startTime) / 1000;
            const remainingSteps = jobStatus.totalSteps - jobStatus.currentStep;
            if (remainingSteps > 0 && jobStatus.stepTimeMs > 0) {
              jobStatus.remainingTimeS = remainingSteps * (jobStatus.stepTimeMs / 1000);
            } else {
              jobStatus.remainingTimeS = 0;
            }
          }
        });

        const rlErr = readline.createInterface({ input: currentJob.stderr });
        rlErr.on('line', (line) => {
          appendLog(`[ERROR] ${line}`);
        });

        currentJob.on('error', (err) => {
          jobStatus.status = 'failed';
          jobStatus.errorMsg = '啟動程式失敗: ' + err.message;
          jobStatus.endTime = Date.now();
          currentJob = null;
        });

        currentJob.on('close', (code) => {
          jobStatus.endTime = Date.now();
          if (stopChanTriggered) {
            jobStatus.status = 'terminated';
          } else if (code !== 0) {
            jobStatus.status = 'failed';
            jobStatus.errorMsg = `執行檔異常退出，代碼: ${code}`;
          } else {
            jobStatus.status = 'completed';
            jobStatus.currentStep = jobStatus.totalSteps;
            jobStatus.remainingTimeS = 0;
          }

          // 任務結束時，優先把最終的預覽圖讀入記憶體緩存
          const previewImgPath = path.join(__dirname, 'img_preview', 'web_preview.png');
          if (fs.existsSync(previewImgPath)) {
            try {
              lastPreviewBuffer = fs.readFileSync(previewImgPath);
            } catch (readErr) {
              console.error('快取最終圖片失敗:', readErr);
            }
          }

          // 清理整個 img_preview 目錄內的所有實體檔案
          const previewDir = path.join(__dirname, 'img_preview');
          if (fs.existsSync(previewDir)) {
            try {
              const files = fs.readdirSync(previewDir);
              for (const file of files) {
                fs.unlinkSync(path.join(previewDir, file));
              }
            } catch (cleanErr) {
              console.error('清理 img_preview 暫存失敗:', cleanErr);
            }
          }

          currentJob = null;
        });

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ status: 'started' }));

      } catch (err) {
        res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
        res.end('無效的 JSON');
      }
    });
    return;
  }

  // ----------------------------------------------------
  // 狀態查詢 API
  // ----------------------------------------------------
  if (req.method === 'GET' && pathname === '/api/status') {
    if (jobStatus.status === 'running') {
      jobStatus.elapsedTimeS = (Date.now() - jobStatus.startTime) / 1000;
      const remainingSteps = jobStatus.totalSteps - jobStatus.currentStep;
      if (remainingSteps > 0 && jobStatus.stepTimeMs > 0) {
        jobStatus.remainingTimeS = remainingSteps * (jobStatus.stepTimeMs / 1000);
      }
    } else if (jobStatus.status === 'completed' || jobStatus.status === 'terminated' || jobStatus.status === 'failed') {
      if (jobStatus.startTime && jobStatus.endTime) {
        jobStatus.elapsedTimeS = (jobStatus.endTime - jobStatus.startTime) / 1000;
      }
      jobStatus.remainingTimeS = 0;
    }

    res.writeHead(200, {
      'Content-Type': 'application/json',
      'Cache-Control': 'no-store, no-cache, must-revalidate, proxy-revalidate',
      'Pragma': 'no-cache',
      'Expires': '0'
    });
    res.end(JSON.stringify(jobStatus));
    return;
  }

  // ----------------------------------------------------
  // 中止任務 API
  // ----------------------------------------------------
  if (req.method === 'POST' && pathname === '/api/stop') {
    if (jobStatus.status !== 'running' || !currentJob) {
      res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('目前沒有正在運行的任務');
      return;
    }

    stopChanTriggered = true;
    currentJob.kill('SIGKILL'); // 強制結束進程

    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'stopping' }));
    return;
  }

  // ----------------------------------------------------
  // 已生成 JSON 清單 API
  // ----------------------------------------------------
  if (req.method === 'GET' && pathname === '/api/json-list') {
    const jsonDir = path.join(__dirname, 'img_json');
    fs.readdir(jsonDir, (err, files) => {
      if (err) {
        if (err.code === 'ENOENT') {
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ status: 'success', files: [] }));
          return;
        }
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ status: 'error', message: err.message }));
        return;
      }
      const jsonFiles = files.filter(f => f.toLowerCase().endsWith('.json'));
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'success', files: jsonFiles }));
    });
    return;
  }

  // ----------------------------------------------------
  // 外部 JSON 上傳 API (流式接收)
  // ----------------------------------------------------
  if (req.method === 'POST' && pathname === '/api/inject-upload') {
    const rawFilename = req.headers['x-filename'] || 'upload.json';
    const filename = decodeURIComponent(rawFilename);
    const ext = path.extname(filename);
    const base = path.basename(filename, ext);
    
    const safeName = `${base}_${Date.now()}.json`;
    const targetPath = path.join(__dirname, 'img_json', safeName);

    // 確保 target 資料夾存在
    const jsonDir = path.dirname(targetPath);
    if (!fs.existsSync(jsonDir)) {
      fs.mkdirSync(jsonDir, { recursive: true });
    }

    const writeStream = fs.createWriteStream(targetPath);
    req.pipe(writeStream);

    let responded = false;
    const sendError = (errMessage) => {
      if (responded) return;
      responded = true;
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'error', message: errMessage }));
    };

    req.on('error', (err) => {
      sendError(err.message);
    });

    writeStream.on('error', (err) => {
      sendError(err.message);
    });

    writeStream.on('finish', () => {
      if (responded) return;
      responded = true;
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'success', filename: safeName }));
    });
    return;
  }

  // ----------------------------------------------------
  // 串流注入日誌 API (Server-Sent Events)
  // ----------------------------------------------------
  if (req.method === 'GET' && pathname === '/api/inject-stream') {
    const filename = parsedUrl.searchParams.get('file');
    if (!filename) {
      res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('缺少 file 參數');
      return;
    }

    const safeFilename = path.basename(filename);
    const jsonPath = path.join(__dirname, 'img_json', safeFilename);

    if (!fs.existsSync(jsonPath)) {
      res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('找不到指定的 JSON 檔案');
      return;
    }

    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
      'X-Accel-Buffering': 'no'
    });

    const pythonScript = path.join(__dirname, 'tools', 'fh6_import_layer_table.py');
    const pyProcess = spawn('python', [pythonScript, '--json', jsonPath], { cwd: __dirname });

    let responseEnded = false;
    const endResponse = (code) => {
      if (responseEnded) return;
      responseEnded = true;
      res.write(`data: ${JSON.stringify({ done: true, code })}\n\n`);
      res.end();
    };

    const sendLog = (data) => {
      if (responseEnded) return;
      const lines = data.toString().split('\n');
      lines.forEach(line => {
        if (line.trim()) {
          res.write(`data: ${JSON.stringify({ log: line.trim() })}\n\n`);
        }
      });
    };

    pyProcess.stdout.on('data', sendLog);
    pyProcess.stderr.on('data', sendLog);

    pyProcess.on('error', (err) => {
      if (!responseEnded) {
        res.write(`data: ${JSON.stringify({ log: `[ERROR] 無法啟動 Python 注入進程 (請確認本機已安裝 Python 並將其加入環境變數): ${err.message}` })}\n\n`);
      }
      endResponse(-1);
    });

    pyProcess.on('close', (code) => {
      endResponse(code);
    });

    req.on('close', () => {
      if (!responseEnded) {
        pyProcess.kill('SIGKILL');
      }
    });

    return;
  }

  // ----------------------------------------------------
  // 404 未找到
  // ----------------------------------------------------
  res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end('路徑不存在');
});

server.listen(PORT, () => {
  console.log(`Geometrize Web Server 正在執行於 http://localhost:${PORT}`);
});
