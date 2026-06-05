import cv2
import numpy as np
from pipeline import PipelineContext, StageResult

class StyleStage:
    def run(self, ctx: PipelineContext, params: dict) -> StageResult:
        method = params.get("method", "bilateral")
        
        if method == "toon":
            result_img = self._toon(ctx.image, params)
            return StageResult(success=True, image=result_img, message="Applied Toon filter")
        else:
            result_img = self._bilateral(ctx.image, params)
            return StageResult(success=True, image=result_img, message=f"Applied Bilateral filter (iterations={params.get('iterations', 2)})")

    def _bilateral(self, img: np.ndarray, params: dict) -> np.ndarray:
        d = params.get("bilateral_d", 9)
        sc = params.get("bilateral_sigma_color", 75)
        ss = params.get("bilateral_sigma_space", 75)
        iterations = params.get("iterations", 2)

        result = img.copy()
        
        if len(img.shape) == 3 and img.shape[2] == 4:
            bgr = img[:, :, :3]
            alpha = img[:, :, 3]
            for _ in range(iterations):
                bgr = cv2.bilateralFilter(bgr, d, sc, ss)
            result[:, :, :3] = bgr
        else:
            for _ in range(iterations):
                result = cv2.bilateralFilter(result, d, sc, ss)
        return result

    def _toon(self, img: np.ndarray, params: dict) -> np.ndarray:
        is_bgra = len(img.shape) == 3 and img.shape[2] == 4
        bgr = img[:, :, :3] if is_bgra else img

        gray = cv2.cvtColor(bgr, cv2.COLOR_BGR2GRAY)
        gray_blur = cv2.medianBlur(gray, 3)
        edges = cv2.adaptiveThreshold(
            gray_blur, 255,
            cv2.ADAPTIVE_THRESH_MEAN_C,
            cv2.THRESH_BINARY, 9, 2
        )
        color = cv2.bilateralFilter(bgr, 7, 50, 50)
        cartoon = cv2.bitwise_and(color, color, mask=edges)

        if is_bgra:
            result = img.copy()
            result[:, :, :3] = cartoon
            return result
        return cartoon
