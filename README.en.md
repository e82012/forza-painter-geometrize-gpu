# Forza Painter Geometrize GPU Version

**Forza is a trademark of Microsoft. This project is unofficial and is not affiliated with or endorsed by Microsoft.**

This is a third-party Geometrize shape (JSON) generation tool based on [forza-painter](https://github.com/forza-painter/forza-painter). It leverages GPU acceleration and modern algorithm enhancements to greatly improve image fitting quality and processing speed.

* [原始繁體中文 README 說明 (README.md)](README.md)
* [原始簡體中文 README 說明 (備份)](README.original.md)

---

## 🚀 Optimizations & Enhancements in `feature/edge-guided-sampling`

The `feature/edge-guided-sampling` branch introduces the following adjustments and optimizations:

### 1. CMA-ES Optimization Algorithm (Covariance Matrix Adaptation Evolution Strategy)
* **Description**: Uses CMA-ES to search the 5D parameter space (center $X, Y$, semi-axes $RX, RY$, and rotation angle $\theta$) for ellipses.
* **Note**: Provides an alternative multi-dimensional parameter search strategy alongside the original hill-climb algorithm to adjust fitting details.

### 2. Edge-Guided Importance Sampling
* **Description**: Generates an "Edge Map" using a Sobel filter at startup. When evaluating candidate shape pixel errors on the GPU, pixels located on strong edges are assigned extra error weights.
* **Note**: Guides geometric shapes toward high-detail regions (such as lines and contours) to improve edge alignment.

### 3. Multi-Scale Hierarchical Fitting
* **Description**: Introduces a coarse-to-fine progressive fitting workflow.
* **Note**: Renders large, basic geometric blocks at lower resolutions first, then progressively increases resolution and fits smaller ellipses for detail refinement.

### 4. Web UI Dashboard & Real-Time Logs
* **Description**: Provides a Node.js-based web interface to load images, configure parameters, dynamically preview the canvas, and monitor Go engine logs and computation times.

### 5. Ring Buffer Pipelining Optimization
* **Description**: Uses a dual-buffer/ring-buffer asynchronous execution pipeline (`ringSize = 3`) to allow the CPU to perform CMA-ES sampling while the GPU concurrently applies the previous shape and computes the error grid, improving parallel execution efficiency.


---

## 🛠️ Build & Installation

### Requirements
* Go w/ CGO >= v1.24
* OpenCL-SDK >= v3.0.19

### Build on Windows
1. Clone the repository.
2. Download the Windows release of [OpenCL-SDK](https://github.com/KhronosGroup/OpenCL-SDK/releases/tag/v2025.07.23) and place it under the `/OpenCL-SDK` directory.
3. Run the build script in PowerShell:
   ```powershell
   powershell -ExecutionPolicy Bypass -File "build-opencl.ps1"
   ```

---

## 💻 Usage

### Command Line Arguments

```bash
Usage: forza-painter-geometrize-go.exe [--settings path.ini|--profile name] [--output path] [--preview path] [--seed n] [--edge-weight w] [--multiscale] [--save-pass-previews] <image-path>
```

| Argument | Description | Default |
| :--- | :--- | :--- |
| `--settings` | Path to the settings `.ini` file | None |
| `--profile` | Profile name fragment under `./settings` | None |
| `--output` | Output path prefix for generated JSON shapes | Input image path |
| `--preview` | Output path for the real-time preview PNG | None |
| `--seed` | RNG seed for reproducible output | `0` |
| `--edge-weight` | Edge-guided sampling weight (`0` to disable, recommended: `2.0` ~ `5.0`) | `-1.0` (disabled) |
| `--multiscale` | Enable multi-scale hierarchical fitting (coarse-to-fine) | `false` |
| `--save-pass-previews` | Save a preview image at the end of each hierarchical pass | `false` |

### CLI Example

* **Run with OpenCL acceleration, edge-guided sampling, and multi-scale fitting**:
  ```cmd
  forza-painter-geometrize-go.exe C:\work\forza\test.png --settings "C:\work\forza\settings\c.ini" --preview "C:\work\forza\preview.png" --edge-weight 3.0 --multiscale
  ```

---

## 🌐 Web UI Usage Guide

This branch provides a browser-based dashboard to easily operate the engine and monitor logs and canvas progress.

### Getting Started
1. Ensure the compiled Go executable is named `forza-painter-geometrize-go-v1.0.exe` and placed in the project root directory (otherwise, edit the `execPath` configuration in `server.js`).
2. Run the Node.js server by double-clicking `start_server.bat` or executing:
   ```bash
   node server.js
   ```
3. Once started, the dashboard will open automatically in your default browser at `http://localhost:8080` (if not, open it manually).
4. Upload an image, choose a quality profile (Low, Medium, High) or customize parameters, and click "Start" to see the canvas updates and engine logs in real-time.

