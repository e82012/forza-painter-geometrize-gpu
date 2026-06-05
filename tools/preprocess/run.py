import argparse
import sys
import cv2
import yaml
from pathlib import Path

from classifier import ImageClassifier
from pipeline import PipelineContext, PipelineRunner
from upscale import UpscaleStage
from style import StyleStage
from quantize import QuantizeStage
from edge import EdgeEnhanceStage
from writer import OutputWriter

STAGE_MAP = {
    "upscale": UpscaleStage,
    "style": StyleStage,
    "quantize": QuantizeStage,
    "edge_enhance": EdgeEnhanceStage,
}

def load_profile(profile_name: str) -> dict:
    profile_path = Path(__file__).parent / "profiles" / f"{profile_name}.yaml"
    if not profile_path.exists():
        raise FileNotFoundError(f"Profile {profile_name} not found at {profile_path}")
    with open(profile_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)

def apply_cli_overrides(profile: dict, args):
    stages = profile.get("stages", [])
    for stage in stages:
        if args.no_upscale and stage["name"] == "upscale":
            stage["enabled"] = False
        if args.no_style and stage["name"] == "style":
            stage["enabled"] = False
        if args.no_quantize and stage["name"] == "quantize":
            stage["enabled"] = False

def derive_output_path(input_path: str) -> Path:
    p = Path(input_path)
    return p.with_name(f"{p.stem}_preprocessed.png")

def main():
    parser = argparse.ArgumentParser(description="AI Preprocess Stylization Pipeline")
    parser.add_argument("--input", required=True, help="Input image path")
    parser.add_argument("--output", help="Output image path")
    parser.add_argument("--profile", default="auto", help="Profile to use: auto, portrait, logo, illustration, landscape")
    parser.add_argument("--no-upscale", action="store_true", help="Skip upscale stage")
    parser.add_argument("--no-style", action="store_true", help="Skip style stage")
    parser.add_argument("--no-quantize", action="store_true", help="Skip quantize stage")
    parser.add_argument("--dry-run", action="store_true", help="Print decisions and exit without processing")
    
    args = parser.parse_args()
    
    input_p = Path(args.input)
    if not input_p.exists():
        print(f"Error: Input file {args.input} does not exist.")
        sys.exit(1)

    img_bgr = cv2.imread(str(input_p), cv2.IMREAD_UNCHANGED)
    if img_bgr is None:
        print(f"Error: Failed to load image {args.input}")
        sys.exit(1)

    classifier = ImageClassifier()
    image_type = args.profile
    if image_type == "auto":
        image_type = classifier.classify(img_bgr)
    print(f"[Classifier] Detected/Selected type: {image_type}")

    if args.dry_run:
        print(f"[Dry Run] Would use profile: {image_type}")
        return

    profile = load_profile(image_type)
    apply_cli_overrides(profile, args)

    out_path = Path(args.output) if args.output else derive_output_path(args.input)
    has_alpha = len(img_bgr.shape) == 3 and img_bgr.shape[2] == 4

    ctx = PipelineContext(
        input_path=input_p,
        output_path=out_path,
        profile_name=image_type,
        image=img_bgr.copy(),
        original=img_bgr.copy(),
        has_alpha=has_alpha,
    )

    runner = PipelineRunner(profile, STAGE_MAP)
    try:
        ctx = runner.run(ctx)
    except Exception as e:
        print(f"[Error] Pipeline failed: {e}")
        sys.exit(1)

    writer = OutputWriter()
    result_path = writer.write(ctx)
    
    print(f"[Done] Preprocessed image saved to: {result_path}")
    for log_line in ctx.log:
        print(f"  {log_line}")

if __name__ == "__main__":
    main()
