import cv2
from pathlib import Path
from pipeline import PipelineContext

class OutputWriter:
    def write(self, ctx: PipelineContext) -> Path:
        out_img_path = ctx.output_path
        
        cv2.imwrite(str(out_img_path), ctx.image)

        log_path = out_img_path.with_suffix(".preprocess.log")
        log_path.write_text("\n".join(ctx.log), encoding='utf-8')
        
        return out_img_path
