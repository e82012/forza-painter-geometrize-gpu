import cv2
import numpy as np

class ImageClassifier:
    def classify(self, img: np.ndarray) -> str:
        metrics = self._compute_metrics(img)
        
        if metrics["has_alpha"] and metrics["edge_density"] > 0.05:
            return "logo"
        if metrics["unique_colors"] <= 64:
            return "logo"
        if metrics["skin_ratio"] > 0.15:
            return "portrait"
        if metrics["saturation_median"] < 0.3 and metrics["edge_density"] < 0.08:
            return "landscape"
            
        return "illustration"

    def _compute_metrics(self, img: np.ndarray) -> dict:
        img_bgr = img[:, :, :3] if len(img.shape) == 3 and img.shape[2] >= 3 else img
        return {
            "has_alpha": len(img.shape) == 3 and img.shape[2] == 4,
            "edge_density": self._sobel_density(img_bgr),
            "skin_ratio": self._skin_pixel_ratio(img_bgr),
            "unique_colors": self._quantized_unique(img_bgr),
            "saturation_median": self._hsv_sat_median(img_bgr),
        }

    def _sobel_density(self, img_bgr: np.ndarray) -> float:
        gray = cv2.cvtColor(img_bgr, cv2.COLOR_BGR2GRAY)
        gx = cv2.Sobel(gray, cv2.CV_32F, 1, 0, ksize=3)
        gy = cv2.Sobel(gray, cv2.CV_32F, 0, 1, ksize=3)
        mag = cv2.magnitude(gx, gy)
        edges = mag > 50
        return np.count_nonzero(edges) / (img_bgr.shape[0] * img_bgr.shape[1])

    def _skin_pixel_ratio(self, img_bgr: np.ndarray) -> float:
        hsv = cv2.cvtColor(img_bgr, cv2.COLOR_BGR2HSV)
        mask1 = cv2.inRange(hsv, (0, 51, 102), (20, 204, 255))
        mask2 = cv2.inRange(hsv, (340, 51, 102), (360, 204, 255))
        return np.count_nonzero(mask1 | mask2) / (img_bgr.shape[0] * img_bgr.shape[1])

    def _quantized_unique(self, img_bgr: np.ndarray) -> int:
        quantized = (img_bgr // 32) * 32
        pixels = quantized.reshape(-1, 3)
        unique_colors = np.unique(pixels, axis=0)
        return len(unique_colors)

    def _hsv_sat_median(self, img_bgr: np.ndarray) -> float:
        hsv = cv2.cvtColor(img_bgr, cv2.COLOR_BGR2HSV)
        sat = hsv[:, :, 1]
        return np.median(sat) / 255.0
