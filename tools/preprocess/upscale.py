import cv2
import numpy as np
from pipeline import PipelineContext, StageResult

class UpscaleStage:
    def run(self, ctx: PipelineContext, params: dict) -> StageResult:
        method = params.get("method", "lanczos")
        scale = params.get("scale", 2)
        
        if scale <= 1:
            return StageResult(success=True, image=ctx.image, message="Scale <= 1, skipped")

        # Phase 1: Only support Lanczos fallback
        return self._run_lanczos_fallback(ctx.image, params)

    def _run_lanczos_fallback(self, img: np.ndarray, params: dict) -> StageResult:
        scale = params.get("scale", 2)
        h, w = img.shape[:2]
        resized = cv2.resize(
            img,
            (int(w * scale), int(h * scale)),
            interpolation=cv2.INTER_LANCZOS4
        )
        return StageResult(success=True, image=resized, message=f"Lanczos x{scale}: {w}x{h} -> {int(w*scale)}x{int(h*scale)}")
