import cv2
import numpy as np
from pipeline import PipelineContext, StageResult

class QuantizeStage:
    def run(self, ctx: PipelineContext, params: dict) -> StageResult:
        method = params.get("method", "kmeans")
        
        if method == "posterize":
            result_img = self._posterize(ctx.image, params)
            return StageResult(success=True, image=result_img, message=f"Applied Posterize (bits={params.get('bits', 4)})")
        else:
            result_img = self._kmeans(ctx.image, params)
            return StageResult(success=True, image=result_img, message=f"Applied K-Means quantization (n_colors={params.get('n_colors', 16)})")

    def _kmeans(self, img: np.ndarray, params: dict) -> np.ndarray:
        n_colors = params.get("n_colors", 16)
        colorspace = params.get("colorspace", "lab")

        is_bgra = len(img.shape) == 3 and img.shape[2] == 4
        bgr = img[:, :, :3] if is_bgra else img

        h, w = bgr.shape[:2]
        pixels = bgr.reshape(-1, 3).astype(np.float32)

        if colorspace == "lab":
            lab_img = cv2.cvtColor(bgr, cv2.COLOR_BGR2LAB)
            pixels = lab_img.reshape(-1, 3).astype(np.float32)

        criteria = (cv2.TERM_CRITERIA_EPS + cv2.TERM_CRITERIA_MAX_ITER, 20, 1.0)
        _, labels, centers = cv2.kmeans(
            pixels, n_colors, None, criteria, 5, cv2.KMEANS_PP_CENTERS
        )
        quantized = centers[labels.flatten()].reshape(h, w, 3).astype(np.uint8)

        if colorspace == "lab":
            quantized = cv2.cvtColor(quantized, cv2.COLOR_LAB2BGR)
            
        if is_bgra:
            result = img.copy()
            result[:, :, :3] = quantized
            return result
        return quantized

    def _posterize(self, img: np.ndarray, params: dict) -> np.ndarray:
        bits = params.get("bits", 4)
        shift = 8 - bits
        
        is_bgra = len(img.shape) == 3 and img.shape[2] == 4
        bgr = img[:, :, :3] if is_bgra else img
        
        quantized = ((bgr >> shift) << shift)
        
        if is_bgra:
            result = img.copy()
            result[:, :, :3] = quantized
            return result
        return quantized
