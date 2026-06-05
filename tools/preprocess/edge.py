import cv2
import numpy as np
from pipeline import PipelineContext, StageResult

class EdgeEnhanceStage:
    def run(self, ctx: PipelineContext, params: dict) -> StageResult:
        method = params.get("method", "unsharp_mask")
        
        if method == "unsharp_mask":
            result_img = self._unsharp_mask(ctx.image, params)
            return StageResult(success=True, image=result_img, message=f"Applied Unsharp Mask (amount={params.get('amount', 1.5)})")
        
        return StageResult(success=True, image=ctx.image, message="Edge enhancement skipped or unknown method")

    def _unsharp_mask(self, img: np.ndarray, params: dict) -> np.ndarray:
        amount = params.get("amount", 1.5)
        radius = params.get("radius", 1.0)
        threshold = params.get("threshold", 5)

        is_bgra = len(img.shape) == 3 and img.shape[2] == 4
        bgr = img[:, :, :3] if is_bgra else img

        blurred = cv2.GaussianBlur(bgr, (0, 0), radius)
        sharpened = cv2.addWeighted(bgr, 1 + amount, blurred, -amount, 0)

        diff = np.abs(bgr.astype(int) - blurred.astype(int)).max(axis=2)
        mask = diff > threshold
        
        result_bgr = bgr.copy()
        result_bgr[mask] = sharpened[mask]
        
        if is_bgra:
            result = img.copy()
            result[:, :, :3] = result_bgr
            return result
        return result_bgr
