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

    // 本地預覽
    const reader = new FileReader();
    reader.onload = (e) => {
      uploadPreview.src = e.target.result;
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
        btnStart.disabled = false;
        showToast('圖片上傳成功，準備擬合！', 'success');
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
    uploadPrompt.classList.remove('hidden');
    uploadPreviewContainer.classList.add('hidden');
    dropZone.classList.remove('has-image');
    btnStart.disabled = true;
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
});
