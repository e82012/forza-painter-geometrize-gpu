from dataclasses import dataclass, field
import numpy as np
from pathlib import Path

@dataclass
class PipelineContext:
    input_path: Path
    output_path: Path
    profile_name: str

    image: np.ndarray
    original: np.ndarray
    has_alpha: bool = False

    image_type: str = "auto"
    log: list[str] = field(default_factory=list)
    stage_outputs: dict = field(default_factory=dict)

@dataclass
class StageResult:
    success: bool
    image: np.ndarray
    message: str = ""
    skipped: bool = False

class PipelineRunner:
    def __init__(self, profile: dict, stage_map: dict):
        self.profile = profile
        self.stage_map = stage_map

    def run(self, ctx: PipelineContext) -> PipelineContext:
        for stage_cfg in self.profile.get("stages", []):
            if not stage_cfg.get("enabled", True):
                ctx.log.append(f"[SKIP] {stage_cfg['name']}")
                continue

            stage_cls = self.stage_map.get(stage_cfg["name"])
            if stage_cls is None:
                raise ValueError(f"Unknown stage: {stage_cfg['name']}")

            stage = stage_cls()
            result = stage.run(ctx, stage_cfg.get("params", {}))

            if result.skipped:
                ctx.log.append(f"[SKIP] {stage_cfg['name']}: {result.message}")
            elif result.success:
                ctx.image = result.image
                ctx.log.append(f"[OK]   {stage_cfg['name']}: {result.message}")
            else:
                raise RuntimeError(f"Stage {stage_cfg['name']} failed: {result.message}")

        return ctx
