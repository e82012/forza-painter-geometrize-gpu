document.addEventListener('DOMContentLoaded', () => {
  // DOM 元素選取
  const dropZone = document.getElementById('dropZone');
  const fileInput = document.getElementById('fileInput');
  const uploadPreviewContainer = document.getElementById('uploadPreviewContainer');
  const uploadPreview = document.getElementById('uploadPreview');
  const btnRemoveFile = document.getElementById('btnRemoveFile');
  const filenameText = document.getElementById('filenameText');
  const uploadPrompt = dropZone.querySelector('.upload-prompt');
  
  const qualityButtons = document.querySelectorAll('.btn-quality');
  const customSettingsPanel = document.getElementById('customSettingsPanel');
  const btnStart = document.getElementById('btnStart');
  const btnStop = document.getElementById('btnStop');
  
  const statusDot = document.getElementById('statusDot');
  const statusText = document.getElementById('statusText');
  const placeholderPrompt = document.getElementById('placeholderPrompt');
  const previewImg = document.getElementById('previewImg');
  const loaderOverlay = document.getElementById('loaderOverlay');
  
  const progressPercent = document.getElementById('progressPercent');
  const progressFill = document.getElementById('progressFill');
  
  const statLayers = document.getElementById('statLayers');
  const statElapsedTime = document.getElementById('statElapsedTime');
  const statRemainingTime = document.getElementById('statRemainingTime');
  const statStepTime = document.getElementById('statStepTime');
  const toastContainer = document.getElementById('toastContainer');
  const consoleLogBox = document.getElementById('consoleLogBox');

  // 自定義參數欄位
  const stopAtInput = document.getElementById('stopAt');
  const maxResolutionInput = document.getElementById('maxResolution');
  const mutatedSamplesInput = document.getElementById('mutatedSamples');
  const randomSamplesInput = document.getElementById('randomSamples');

  // 狀態變數
  let uploadedFilename = '';
  let selectedQuality = 'low';
  let pollIntervalId = null;
  let jobStartTime = null;
  let selectedFileObject = null;
  let originalImageSrc = '';
  let preprocessedImageBlob = null;

  // 請求 Notification 權限
  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission();
  }

  // ----------------------------------------------------
  // Drag & Drop 上傳處理
  // ----------------------------------------------------
  
  // 點擊上傳區觸發檔案選擇器
  dropZone.addEventListener('click', (e) => {
    if (e.target !== btnRemoveFile && !btnRemoveFile.contains(e.target)) {
      fileInput.click();
    }
  });

  // 鍵盤無障礙支援
  dropZone.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      fileInput.click();
    }
  });

  // 拖曳狀態樣式
  ['dragenter', 'dragover'].forEach(eventName => {
    dropZone.addEventListener(eventName, (e) => {
      e.preventDefault();
      dropZone.classList.add('dragover');
    }, false);
  });

  ['dragleave', 'drop'].forEach(eventName => {
    dropZone.addEventListener(eventName, (e) => {
      e.preventDefault();
      dropZone.classList.remove('dragover');
    }, false);
  });

  dropZone.addEventListener('drop', (e) => {
    const dt = e.dataTransfer;
    const files = dt.files;
    if (files.length > 0) {
      handleFileSelect(files[0]);
    }
  });

  fileInput.addEventListener('change', (e) => {
    if (fileInput.files.length > 0) {
      handleFileSelect(fileInput.files[0]);
    }
  });

  // 移除已選擇檔案
  btnRemoveFile.addEventListener('click', (e) => {
    e.stopPropagation();
    resetUploadZone();
  });

  function handleFileSelect(file) {
    if (!file.type.startsWith('image/')) {
      showToast('請上傳圖片檔案！', 'error');
      return;
    }

    selectedFileObject = file;

    // 本地預覽
    const reader = new FileReader();
    reader.onload = (e) => {
      uploadPreview.src = e.target.result;
      originalImageSrc = e.target.result;
      uploadPrompt.classList.add('hidden');
      uploadPreviewContainer.classList.remove('hidden');
      dropZone.classList.add('has-image');
    };
    reader.readAsDataURL(file);

    // 上傳檔案至後端
    uploadFile(file);
  }

  function uploadFile(file) {
    showToast('正在上傳圖片...', 'info');
    btnStart.disabled = true;

    fetch('/api/upload', {
      method: 'POST',
      headers: {
        'Content-Type': file.type,
        'X-Filename': encodeURIComponent(file.name)
      },
      body: file
    })
    .then(res => {
      if (!res.ok) throw new Error('上傳失敗');
      return res.json();
    })
    .then(data => {
      if (data.status === 'success') {
        uploadedFilename = data.filename;
        showToast('圖片上傳成功！', 'success');

        const enablePreprocess = document.getElementById('enablePreprocess');
        if (enablePreprocess && enablePreprocess.checked) {
          applyPreprocessing();
        } else {
          btnStart.disabled = false;
        }
      } else {
        throw new Error(data.message || '上傳失敗');
      }
    })
    .catch(err => {
      showToast('圖片上傳失敗: ' + err.message, 'error');
      resetUploadZone();
    });
  }

  function resetUploadZone() {
    fileInput.value = '';
    uploadedFilename = '';
    uploadPreview.src = '';
    selectedFileObject = null;
    originalImageSrc = '';
    preprocessedImageBlob = null;
    uploadPrompt.classList.remove('hidden');
    uploadPreviewContainer.classList.add('hidden');
    dropZone.classList.remove('has-image');
    btnStart.disabled = true;

    const enablePreprocess = document.getElementById('enablePreprocess');
    if (enablePreprocess) {
      enablePreprocess.checked = false;
      document.getElementById('preprocessControls').classList.add('collapsed');
    }
  }

  // ----------------------------------------------------
  // 圖片預處理邏輯
  // ----------------------------------------------------
  const enablePreprocess = document.getElementById('enablePreprocess');
  const preprocessControls = document.getElementById('preprocessControls');
  const preprocessSmooth = document.getElementById('preprocessSmooth');
  const preprocessSmoothVal = document.getElementById('preprocessSmoothVal');
  const preprocessPosterize = document.getElementById('preprocessPosterize');
  const preprocessPosterizeVal = document.getElementById('preprocessPosterizeVal');
  const btnApplyPreprocess = document.getElementById('btnApplyPreprocess');

  if (enablePreprocess) {
    enablePreprocess.addEventListener('change', () => {
      if (enablePreprocess.checked) {
        preprocessControls.classList.remove('collapsed');
        if (originalImageSrc) {
          applyPreprocessing();
        }
      } else {
        preprocessControls.classList.add('collapsed');
        if (originalImageSrc) {
          uploadPreview.src = originalImageSrc;
          preprocessedImageBlob = null;
          // 恢復原本的圖片檔案到後端
          if (selectedFileObject) {
            uploadFile(selectedFileObject);
          }
        }
      }
    });
  }

  if (preprocessSmooth) {
    preprocessSmooth.addEventListener('input', () => {
      preprocessSmoothVal.textContent = preprocessSmooth.value + 'px';
    });
  }

  if (preprocessPosterize) {
    preprocessPosterize.addEventListener('input', () => {
      preprocessPosterizeVal.textContent = preprocessPosterize.value + ' 色';
    });
  }

  if (btnApplyPreprocess) {
    btnApplyPreprocess.addEventListener('click', () => {
      applyPreprocessing();
    });
  }

  function applyPreprocessing() {
    if (!originalImageSrc) {
      showToast('請先上傳圖片！', 'warning');
      return;
    }
    showToast('正在預處理圖像...', 'info');
    
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      const canvas = document.createElement('canvas');
      const ctx = canvas.getContext('2d');
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;

      // 1. 設定平滑降噪強度 (Blur)
      const blurRadius = parseFloat(preprocessSmooth.value);
      if (blurRadius > 0) {
        ctx.filter = `blur(${blurRadius}px)`;
      }

      // 2. 繪製原圖 (套用 CSS Filter)
      ctx.drawImage(img, 0, 0);
      ctx.filter = 'none'; // 重置濾鏡

      // 3. 套用色階量化數量 (Posterize)
      const levels = parseInt(preprocessPosterize.value, 10);
      if (levels < 256) {
        const imgData = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const data = imgData.data;
        const numValues = 256 / (levels - 1);
        
        for (let i = 0; i < data.length; i += 4) {
          // R
          data[i] = Math.round(data[i] / numValues) * numValues;
          // G
          data[i+1] = Math.round(data[i+1] / numValues) * numValues;
          // B
          data[i+2] = Math.round(data[i+2] / numValues) * numValues;
        }
        ctx.putImageData(imgData, 0, 0);
      }

      // 4. 更新預覽畫面
      const dataUrl = canvas.toDataURL('image/png');
      uploadPreview.src = dataUrl;

      // 5. 轉換成 Blob 並上傳覆蓋
      canvas.toBlob((blob) => {
        preprocessedImageBlob = blob;
        if (uploadedFilename) {
          uploadPreprocessedBlob();
        }
      }, 'image/png');
    };
    img.src = originalImageSrc;
  }

  function uploadPreprocessedBlob() {
    if (!preprocessedImageBlob || !uploadedFilename) return;
    btnStart.disabled = true;

    const file = new File([preprocessedImageBlob], decodeURIComponent(uploadedFilename), { type: 'image/png' });
    
    fetch('/api/upload', {
      method: 'POST',
      headers: {
        'Content-Type': 'image/png',
        'X-Filename': uploadedFilename
      },
      body: file
    })
    .then(res => {
      if (!res.ok) throw new Error('預處理影像套用失敗');
      return res.json();
    })
    .then(data => {
      if (data.status === 'success') {
        uploadedFilename = data.filename;
        btnStart.disabled = false;
        showToast('預處理影像套用成功！', 'success');
      } else {
        throw new Error(data.message || '套用失敗');
      }
    })
    .catch(err => {
      showToast('套用失敗: ' + err.message, 'error');
      btnStart.disabled = false;
    });
  }

  // ----------------------------------------------------
  // 參數配置與品質選取
  // ----------------------------------------------------
  qualityButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      qualityButtons.forEach(b => {
        b.classList.remove('active');
        b.setAttribute('aria-pressed', 'false');
      });
      btn.classList.add('active');
      btn.setAttribute('aria-pressed', 'true');
      
      selectedQuality = btn.dataset.quality;

      if (selectedQuality === 'custom') {
        customSettingsPanel.classList.remove('collapsed');
      } else {
        customSettingsPanel.classList.add('collapsed');
      }
    });
  });

  // ----------------------------------------------------
  // 開始擬合生成
  // ----------------------------------------------------
  btnStart.addEventListener('click', () => {
    if (!uploadedFilename) return;

    const payload = {
      filename: uploadedFilename,
      quality: selectedQuality
    };

    if (selectedQuality === 'custom') {
      payload.customSettings = {
        stopAt: parseInt(stopAtInput.value, 10),
        maxResolution: parseInt(maxResolutionInput.value, 10),
        mutatedSamples: parseInt(mutatedSamplesInput.value, 10),
        randomSamples: parseInt(randomSamplesInput.value, 10),
        maxPreviewSize: 500
      };
    }

    btnStart.disabled = true;
    loaderOverlay.classList.remove('hidden');
    placeholderPrompt.classList.add('hidden');
    previewImg.classList.remove('hidden');
    previewImg.src = uploadPreview.src;

    // 立即重置所有進度與時間面板，避免呈現上一次任務的舊數據
    progressFill.style.width = '0%';
    progressPercent.textContent = '0%';
    statLayers.textContent = '0 / 0';
    statElapsedTime.textContent = '00:00:00';
    statRemainingTime.textContent = '00:00:00';
    statStepTime.textContent = '0 ms';
    consoleLogBox.innerHTML = '<div class="log-line text-dim">正在啟動幾何擬合引擎...</div>';

    fetch('/api/start', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload)
    })
    .then(res => {
      if (!res.ok) return res.text().then(text => { throw new Error(text) });
      return res.json();
    })
    .then(data => {
      if (data.status === 'started') {
        showToast('擬合任務已啟動！', 'success');
        startPolling();
      }
    })
    .catch(err => {
      showToast('啟動失敗: ' + err.message, 'error');
      btnStart.disabled = false;
      loaderOverlay.classList.add('hidden');
      placeholderPrompt.classList.remove('hidden');
    });
  });

  // ----------------------------------------------------
  // 終止運行
  // ----------------------------------------------------
  btnStop.addEventListener('click', () => {
    btnStop.disabled = true;
    fetch('/api/stop', { method: 'POST' })
      .then(res => {
        if (!res.ok) throw new Error('終止指令失敗');
        showToast('正在要求終止中...', 'info');
      })
      .catch(err => {
        showToast('無法終止: ' + err.message, 'error');
        btnStop.disabled = false;
      });
  });

  // ----------------------------------------------------
  // 輪詢狀態與預覽渲染
  // ----------------------------------------------------
  function startPolling() {
    btnStop.disabled = false;
    btnStart.disabled = true;
    
    // UI 指示器狀態變更
    statusDot.className = 'status-dot running';
    statusText.textContent = '進行中';

    if (pollIntervalId) clearInterval(pollIntervalId);
    
    pollIntervalId = setInterval(checkStatus, 800);
  }

  function stopPolling(finalStatus) {
    if (pollIntervalId) {
      clearInterval(pollIntervalId);
      pollIntervalId = null;
    }
    
    btnStop.disabled = true;
    btnStart.disabled = !uploadedFilename;
    loaderOverlay.classList.add('hidden');

    statusDot.className = `status-dot ${finalStatus}`;
    
    switch (finalStatus) {
      case 'completed':
        statusText.textContent = '已完成';
        progressFill.style.width = '100%';
        progressPercent.textContent = '100%';
        showToast('擬合完成！已將 JSON 與預覽圖存入資料夾。', 'success');
        sendNotification('Forza Geometrize', '圖片幾何擬合完成！');
        break;
      case 'terminated':
        statusText.textContent = '已終止';
        showToast('任務已手動終止。', 'warning');
        break;
      case 'failed':
        statusText.textContent = '出錯';
        break;
      default:
        statusText.textContent = '等待開始';
        statusDot.className = 'status-dot idle';
    }
  }

  function checkStatus() {
    // 加上時間戳參數防瀏覽器快取
    fetch(`/api/status?t=${Date.now()}`)
      .then(res => res.json())
      .then(status => {
        if (status.status !== 'running') {
          // 同步最後的進度與時間數據
          statElapsedTime.textContent = formatTime(status.elapsedTimeS);
          statRemainingTime.textContent = formatTime(status.remainingTimeS);

          if (status.status === 'completed') {
            statLayers.textContent = `${status.totalSteps} / ${status.totalSteps}`;
            progressFill.style.width = '100%';
            progressPercent.textContent = '100%';
            previewImg.classList.remove('hidden');
            loaderOverlay.classList.add('hidden');
            previewImg.src = `/api/preview-image?t=${new Date().getTime()}`;
          }
          stopPolling(status.status);
          if (status.status === 'failed') {
            showToast('擬合失敗: ' + status.errorMsg, 'error');
          }
          return;
        }

        // 更新進度條
        const percent = status.totalSteps > 0 ? (status.currentStep / status.totalSteps) * 100 : 0;
        progressFill.style.width = `${percent}%`;
        progressPercent.textContent = `${Math.round(percent)}%`;

        // 更新效能指標數值
        statLayers.textContent = `${status.currentStep} / ${status.totalSteps}`;
        statElapsedTime.textContent = formatTime(status.elapsedTimeS);
        statRemainingTime.textContent = formatTime(status.remainingTimeS);
        statStepTime.textContent = `${status.stepTimeMs} ms`;

        // 更新即時日誌
        if (status.logs && status.logs.length > 0) {
          const isScrolledToBottom = consoleLogBox.scrollHeight - consoleLogBox.clientHeight <= consoleLogBox.scrollTop + 10;
          consoleLogBox.innerHTML = status.logs.map(line => {
            const isError = line.startsWith('[ERROR]');
            return `<div class="log-line ${isError ? 'text-error' : ''}">${escapeHtml(line)}</div>`;
          }).join('');
          if (isScrolledToBottom) {
            consoleLogBox.scrollTop = consoleLogBox.scrollHeight;
          }
        }

        // 載入預覽圖片 (加上隨機參數避免瀏覽器快取)
        if (status.currentStep > 0) {
          loaderOverlay.classList.add('hidden');
          previewImg.classList.remove('hidden');
          previewImg.src = `/api/preview-image?t=${new Date().getTime()}`;
        }
      })
      .catch(err => {
        console.error('輪詢出錯: ', err);
      });
  }

  // ----------------------------------------------------
  // 輔助工具函式
  // ----------------------------------------------------
  
  // 秒數格式化：hh:mm:ss
  function formatTime(seconds) {
    if (isNaN(seconds) || seconds < 0) return '00:00:00';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);
    return [
      hrs.toString().padStart(2, '0'),
      mins.toString().padStart(2, '0'),
      secs.toString().padStart(2, '0')
    ].join(':');
  }

  // Toast 提示框系統
  function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    
    // 圖標設定
    let icon = 'ℹ️';
    if (type === 'success') icon = '✅';
    if (type === 'error') icon = '❌';
    if (type === 'warning') icon = '⚠️';

    toast.innerHTML = `<span class="toast-icon">${icon}</span><span class="toast-message">${message}</span>`;
    toastContainer.appendChild(toast);

    // 觸發進場動畫
    setTimeout(() => {
      toast.classList.add('show');
    }, 50);

    // 4 秒後自動銷毀
    setTimeout(() => {
      toast.classList.remove('show');
      toast.addEventListener('transitionend', () => {
        toast.remove();
      });
    }, 4000);
  }

  // 瀏覽器桌面通知系統
  function sendNotification(title, body) {
    if ('Notification' in window && Notification.permission === 'granted') {
      new Notification(title, {
        body: body,
        icon: '/favicon.ico'
      });
    }
  }

  // ====================================================
  // 記憶體注入器 DOM 元素與邏輯
  // ====================================================
  const tabBtnGenerator = document.getElementById('tabBtnGenerator');
  const tabBtnInjector = document.getElementById('tabBtnInjector');
  const tabGenerator = document.getElementById('tab-generator');
  const tabInjector = document.getElementById('tab-injector');

  const btnModeSelect = document.getElementById('btnModeSelect');
  const btnModeUpload = document.getElementById('btnModeUpload');
  const modeSelectSection = document.getElementById('modeSelectSection');
  const modeUploadSection = document.getElementById('modeUploadSection');

  const jsonSelect = document.getElementById('jsonSelect');
  const jsonDropZone = document.getElementById('jsonDropZone');
  const jsonFileInput = document.getElementById('jsonFileInput');
  const jsonUploadPreviewContainer = document.getElementById('jsonUploadPreviewContainer');
  const btnRemoveJsonFile = document.getElementById('btnRemoveJsonFile');
  const jsonFilenameText = document.getElementById('jsonFilenameText');
  const jsonUploadPrompt = jsonDropZone ? jsonDropZone.querySelector('.upload-prompt') : null;

  const btnInject = document.getElementById('btnInject');
  const btnStopInject = document.getElementById('btnStopInject');
  const injectorLogConsole = document.getElementById('injectorLogConsole');

  // 注入狀態變數
  let injectEventSource = null;
  let selectedInjectFilename = '';
  let isUploadingJson = false;
  let uploadedJsonFilename = '';

  // ----------------------------------------------------
  // 分頁切換邏輯
  // ----------------------------------------------------
  function switchTab(tabName) {
    if (tabName === 'generator') {
      tabBtnGenerator.classList.add('active');
      tabBtnGenerator.setAttribute('aria-selected', 'true');
      tabBtnInjector.classList.remove('active');
      tabBtnInjector.setAttribute('aria-selected', 'false');

      tabGenerator.classList.add('active');
      tabGenerator.classList.remove('hidden');
      tabInjector.classList.add('hidden');
      tabInjector.classList.remove('active');
    } else {
      tabBtnInjector.classList.add('active');
      tabBtnInjector.setAttribute('aria-selected', 'true');
      tabBtnGenerator.classList.remove('active');
      tabBtnGenerator.setAttribute('aria-selected', 'false');

      tabInjector.classList.add('active');
      tabInjector.classList.remove('hidden');
      tabGenerator.classList.add('hidden');
      tabGenerator.classList.remove('active');

      // 切換至注入分頁時，自動載入本機生成的 JSON 清單
      loadJsonList();
    }
  }

  tabBtnGenerator.addEventListener('click', () => switchTab('generator'));
  tabBtnInjector.addEventListener('click', () => switchTab('injector'));

  // ----------------------------------------------------
  // 載入本機生成的 JSON 檔案清單
  // ----------------------------------------------------
  function loadJsonList() {
    jsonSelect.innerHTML = '<option value="">載入中...</option>';
    fetch('/api/json-list')
      .then(res => res.json())
      .then(data => {
        if (data.status === 'success') {
          if (data.files.length === 0) {
            jsonSelect.innerHTML = '<option value="">(尚未生成任何 JSON 檔案)</option>';
            selectedInjectFilename = '';
            updateInjectButtonState();
            return;
          }

          jsonSelect.innerHTML = '<option value="">-- 請選擇一個幾何 JSON 檔案 --</option>' +
            data.files.map(f => `<option value="${f}">${f}</option>`).join('');

          if (btnModeSelect.classList.contains('active')) {
            selectedInjectFilename = jsonSelect.value;
            updateInjectButtonState();
          }
        } else {
          jsonSelect.innerHTML = '<option value="">無法加載檔案清單</option>';
        }
      })
      .catch(err => {
        console.error('取得 JSON 清單失敗:', err);
        jsonSelect.innerHTML = '<option value="">載入失敗，請重試</option>';
      });
  }

  jsonSelect.addEventListener('change', () => {
    selectedInjectFilename = jsonSelect.value;
    updateInjectButtonState();
  });

  // ----------------------------------------------------
  // 匯入模式切換 (選擇 vs 上傳)
  // ----------------------------------------------------
  btnModeSelect.addEventListener('click', () => {
    btnModeSelect.classList.add('active');
    btnModeUpload.classList.remove('active');
    modeSelectSection.classList.remove('hidden');
    modeUploadSection.classList.add('hidden');

    selectedInjectFilename = jsonSelect.value;
    updateInjectButtonState();
  });

  btnModeUpload.addEventListener('click', () => {
    btnModeUpload.classList.add('active');
    btnModeSelect.classList.remove('active');
    modeUploadSection.classList.remove('hidden');
    modeSelectSection.classList.add('hidden');

    selectedInjectFilename = uploadedJsonFilename;
    updateInjectButtonState();
  });

  // ----------------------------------------------------
  // Drag & Drop 上傳外部 JSON 處理
  // ----------------------------------------------------
  if (jsonDropZone) {
    jsonDropZone.addEventListener('click', (e) => {
      if (e.target !== btnRemoveJsonFile && !btnRemoveJsonFile.contains(e.target)) {
        jsonFileInput.click();
      }
    });

    ['dragenter', 'dragover'].forEach(eventName => {
      jsonDropZone.addEventListener(eventName, (e) => {
        e.preventDefault();
        jsonDropZone.classList.add('dragover');
      }, false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
      jsonDropZone.addEventListener(eventName, (e) => {
        e.preventDefault();
        jsonDropZone.classList.remove('dragover');
      }, false);
    });

    jsonDropZone.addEventListener('drop', (e) => {
      const dt = e.dataTransfer;
      const files = dt.files;
      if (files.length > 0) {
        handleJsonFileSelect(files[0]);
      }
    });
  }

  if (jsonFileInput) {
    jsonFileInput.addEventListener('change', (e) => {
      if (jsonFileInput.files.length > 0) {
        handleJsonFileSelect(jsonFileInput.files[0]);
      }
    });
  }

  if (btnRemoveJsonFile) {
    btnRemoveJsonFile.addEventListener('click', (e) => {
      e.stopPropagation();
      resetJsonUploadZone();
    });
  }

  function handleJsonFileSelect(file) {
    if (!file.name.toLowerCase().endsWith('.json')) {
      showToast('請上傳 JSON 格式的檔案！', 'error');
      return;
    }

    jsonFilenameText.textContent = file.name;
    jsonUploadPrompt.classList.add('hidden');
    jsonUploadPreviewContainer.classList.remove('hidden');
    jsonDropZone.classList.add('has-image');

    uploadJsonFile(file);
  }

  function uploadJsonFile(file) {
    showToast('正在上傳 JSON 貼紙檔案...', 'info');
    btnInject.disabled = true;
    isUploadingJson = true;

    fetch('/api/inject-upload', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Filename': encodeURIComponent(file.name)
      },
      body: file
    })
    .then(res => {
      if (!res.ok) throw new Error('上傳失敗');
      return res.json();
    })
    .then(data => {
      isUploadingJson = false;
      if (data.status === 'success') {
        uploadedJsonFilename = data.filename;
        if (btnModeUpload.classList.contains('active')) {
          selectedInjectFilename = uploadedJsonFilename;
          updateInjectButtonState();
        }
        showToast('JSON 貼紙上傳成功，可開始注入！', 'success');
      } else {
        throw new Error(data.message || '上傳失敗');
      }
    })
    .catch(err => {
      isUploadingJson = false;
      showToast('JSON 上傳失敗: ' + err.message, 'error');
      resetJsonUploadZone();
    });
  }

  function resetJsonUploadZone() {
    jsonFileInput.value = '';
    uploadedJsonFilename = '';
    selectedInjectFilename = '';
    jsonUploadPrompt.classList.remove('hidden');
    jsonUploadPreviewContainer.classList.add('hidden');
    jsonDropZone.classList.remove('has-image');
    updateInjectButtonState();
  }

  function updateInjectButtonState() {
    btnInject.disabled = !selectedInjectFilename || isUploadingJson || injectEventSource !== null;
  }

  // ----------------------------------------------------
  // 記憶體注入執行 (Server-Sent Events 監聽)
  // ----------------------------------------------------
  if (btnInject) {
    btnInject.addEventListener('click', () => {
      if (!selectedInjectFilename) return;

      btnInject.disabled = true;
      btnStopInject.disabled = false;
      
      tabBtnGenerator.disabled = true;
      btnModeSelect.disabled = true;
      btnModeUpload.disabled = true;
      jsonSelect.disabled = true;
      if (jsonDropZone) jsonDropZone.style.pointerEvents = 'none';

      injectorLogConsole.innerHTML = '<div class="log-line text-dim">準備向《極限競速》寫入記憶體...</div>' +
        '<div class="log-line text-dim">正在嘗試尋找遊戲進程與圖層表基底地址...</div>';

      const sseUrl = `/api/inject-stream?file=${encodeURIComponent(selectedInjectFilename)}`;
      injectEventSource = new EventSource(sseUrl);

      injectEventSource.onmessage = (event) => {
        const data = JSON.parse(event.data);

        if (data.log) {
          const isScrolledToBottom = injectorLogConsole.scrollHeight - injectorLogConsole.clientHeight <= injectorLogConsole.scrollTop + 10;
          
          let lineClass = '';
          // 語意化染色
          if (data.log.includes('[ERROR]') || data.log.includes('Failed') || data.log.includes('error') || data.log.toLowerCase().includes('err')) {
            lineClass = 'text-error';
          } else if (data.log.includes('[SUCCESS]') || data.log.includes('Successfully') || data.log.includes('Done') || data.log.includes('寫入完成')) {
            lineClass = 'text-success';
          } else if (data.log.includes('[WARNING]') || data.log.includes('Warning')) {
            lineClass = 'text-warning';
          } else {
            lineClass = 'text-dim';
          }

          const logDiv = document.createElement('div');
          logDiv.className = `log-line ${lineClass}`;
          logDiv.textContent = data.log;
          injectorLogConsole.appendChild(logDiv);

          if (isScrolledToBottom) {
            injectorLogConsole.scrollTop = injectorLogConsole.scrollHeight;
          }
        }

        if (data.done) {
          closeInjectStream();
          if (data.code === 0) {
            showToast('記憶體注入完成！貼紙已即時顯示於遊戲中！', 'success');
            sendNotification('Forza Geometrize', '記憶體車貼注入完成！');
          } else {
            showToast('注入程式中斷，請確認遊戲是否開啟，或是否以管理員權限啟動後端。', 'error');
          }
        }
      };

      injectEventSource.onerror = (err) => {
        console.error('SSE 連線出錯:', err);
        const logDiv = document.createElement('div');
        logDiv.className = 'log-line text-error';
        logDiv.textContent = '[ERROR] 與伺服器的串流連線中斷，注入作業終止。';
        injectorLogConsole.appendChild(logDiv);
        closeInjectStream();
        showToast('注入連線中斷。', 'error');
      };
    });
  }

  if (btnStopInject) {
    btnStopInject.addEventListener('click', () => {
      if (injectEventSource) {
        const logDiv = document.createElement('div');
        logDiv.className = 'log-line text-warning';
        logDiv.textContent = '[WARNING] 注入作業已被使用者手動中止。';
        injectorLogConsole.appendChild(logDiv);
        closeInjectStream();
        showToast('注入作業已手動中止。', 'warning');
      }
    });
  }

  function closeInjectStream() {
    if (injectEventSource) {
      injectEventSource.close();
      injectEventSource = null;
    }
    
    btnStopInject.disabled = true;
    
    tabBtnGenerator.disabled = false;
    btnModeSelect.disabled = false;
    btnModeUpload.disabled = false;
    jsonSelect.disabled = false;
    if (jsonDropZone) jsonDropZone.style.pointerEvents = 'auto';

    updateInjectButtonState();
  }

  function escapeHtml(str) {
    return str.replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
  }
});
