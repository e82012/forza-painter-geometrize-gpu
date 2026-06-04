package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"forza-painter-geometrize-go/internal/model"
)

func DefaultSettings() model.Settings {
	return model.Settings{
		Description:                   "Default profile",
		MaxPreviewSize:                500,
		MaxResolution:                 2000,
		MaxThreads:                    0,
		MutatedSamples:                15000,
		ForceOpaqueShapes:             false,
		PosterizeLevels:               20,
		PreviewEvery:                  10,
		RandomSamples:                 200000,
		SaveAt:                        map[int]struct{}{500: {}, 1000: {}, 1500: {}, 2000: {}, 2500: {}, 3000: {}},
		SaveEvery:                     10,
		StopAt:                        3000,
		UseWorkGroupEval:              true,
		EnableProgressiveSampling:     false,
		ProgressiveSamplingStart:      4,
		ProgressiveSamplingEnd:        1,
		ProgressiveSamplingTransition: 0.333,
		ProgressiveSamplingCurve:      2.5,
		EdgeWeight:                    0.0,
		MultiScale:                    false,
		SavePassPreviews:              false,
		LogoHardEdges:                 false,
		PreprocessMode:                "none",
		EnableTwoStageRandom:          true,
		TwoStageRandomStart:           0.0,
		RandomCoarseSampleStep:        2,
		RandomRefineTopK:              2048,
		EnableLateSmallCandidates:     true,
		LateSmallCandidateShare:       0.66,
		LateSmallCandidateStart:       0.5,
		LateSmallCandidateRadiusFrac:  0.024,
		EnablePruning:                 true,
		PruneThreshold:                0.0000001,
	}
}

func ResolveSettingsPath(rootDir, explicitPath, profile string) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
	}

	settingsDir := filepath.Join(rootDir, "settings")
	if profile != "" {
		matches, err := os.ReadDir(settingsDir)
		if err != nil {
			return "", err
		}
		needle := strings.ToLower(profile)
		for _, entry := range matches {
			if entry.IsDir() {
				continue
			}
			name := strings.ToLower(entry.Name())
			if strings.Contains(name, needle) && strings.HasSuffix(name, ".ini") {
				return filepath.Join(settingsDir, entry.Name()), nil
			}
		}
		return "", fmt.Errorf("profile %q not found in %s", profile, settingsDir)
	}

	return filepath.Join(settingsDir, "_default.ini"), nil
}

func ParseSettings(path string) (model.Settings, error) {
	cfg := DefaultSettings()
	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "description":
			cfg.Description = value
		case "maxPreviewSize":
			cfg.MaxPreviewSize = parseInt(value, cfg.MaxPreviewSize)
		case "maxResolution":
			cfg.MaxResolution = parseInt(value, cfg.MaxResolution)
		case "maxThreads":
			cfg.MaxThreads = parseInt(value, cfg.MaxThreads)
		case "mutatedSamples":
			cfg.MutatedSamples = parseInt(value, cfg.MutatedSamples)
		case "forceOpaqueShapes":
			cfg.ForceOpaqueShapes = parseBool(value, cfg.ForceOpaqueShapes)
		case "posterizeLevels":
			cfg.PosterizeLevels = parseInt(value, cfg.PosterizeLevels)
		case "previewEvery":
			cfg.PreviewEvery = parseInt(value, cfg.PreviewEvery)
		case "randomSamples":
			cfg.RandomSamples = parseInt(value, cfg.RandomSamples)
		case "saveAt":
			cfg.SaveAt = parseSaveAt(value)
		case "saveEvery":
			cfg.SaveEvery = parseInt(value, cfg.SaveEvery)
		case "stopAt":
			cfg.StopAt = parseInt(value, cfg.StopAt)
		case "useWorkGroupEval":
			cfg.UseWorkGroupEval = parseBool(value, cfg.UseWorkGroupEval)
		case "enableProgressiveSampling":
			cfg.EnableProgressiveSampling = parseBool(value, cfg.EnableProgressiveSampling)

		case "progressiveSamplingStart":
			cfg.ProgressiveSamplingStart = parseInt(value, cfg.ProgressiveSamplingStart)

		case "progressiveSamplingEnd":
			cfg.ProgressiveSamplingEnd = parseInt(value, cfg.ProgressiveSamplingEnd)

		case "progressiveSamplingTransition":
			cfg.ProgressiveSamplingTransition = parseFloat(value, cfg.ProgressiveSamplingTransition)

		case "progressiveSamplingCurve":
			cfg.ProgressiveSamplingCurve = parseFloat(value, cfg.ProgressiveSamplingCurve)
		case "edgeWeight":
			cfg.EdgeWeight = float64(parseFloat(value, float32(cfg.EdgeWeight)))
		case "multiScale":
			cfg.MultiScale = parseBool(value, cfg.MultiScale)
		case "savePassPreviews":
			cfg.SavePassPreviews = parseBool(value, cfg.SavePassPreviews)
		case "loadGeometry":
			cfg.LoadGeometry = value
		case "logoHardEdges":
			cfg.LogoHardEdges = parseBool(value, cfg.LogoHardEdges)
		case "preprocessMode":
			cfg.PreprocessMode = value
		case "enableTwoStageRandom":
			cfg.EnableTwoStageRandom = parseBool(value, cfg.EnableTwoStageRandom)
		case "twoStageRandomStart":
			cfg.TwoStageRandomStart = parseFloat(value, cfg.TwoStageRandomStart)
		case "randomCoarseSampleStep":
			cfg.RandomCoarseSampleStep = parseInt(value, cfg.RandomCoarseSampleStep)
		case "randomRefineTopK":
			cfg.RandomRefineTopK = parseInt(value, cfg.RandomRefineTopK)
		case "enableLateSmallCandidates":
			cfg.EnableLateSmallCandidates = parseBool(value, cfg.EnableLateSmallCandidates)
		case "lateSmallCandidateShare":
			cfg.LateSmallCandidateShare = parseFloat(value, cfg.LateSmallCandidateShare)
		case "lateSmallCandidateStart":
			cfg.LateSmallCandidateStart = parseFloat(value, cfg.LateSmallCandidateStart)
		case "lateSmallCandidateRadiusFrac":
			cfg.LateSmallCandidateRadiusFrac = parseFloat(value, cfg.LateSmallCandidateRadiusFrac)
		case "enablePruning":
			cfg.EnablePruning = parseBool(value, cfg.EnablePruning)
		case "pruneThreshold":
			cfg.PruneThreshold = parseFloat64(value, cfg.PruneThreshold)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}

	if cfg.RandomSamples < 1 {
		cfg.RandomSamples = 1
	}
	if cfg.MutatedSamples < 0 {
		cfg.MutatedSamples = 0
	}
	if cfg.StopAt < 1 {
		cfg.StopAt = 1
	}
	if cfg.SaveEvery < 1 {
		cfg.SaveEvery = 1
	}

	if cfg.ProgressiveSamplingStart < 1 {
		cfg.ProgressiveSamplingStart = 1
	}

	if cfg.ProgressiveSamplingEnd < 1 {
		cfg.ProgressiveSamplingEnd = 1
	}

	if cfg.ProgressiveSamplingTransition <= 0 {
		cfg.ProgressiveSamplingTransition = 0.333
	}

	if cfg.ProgressiveSamplingCurve <= 0 {
		cfg.ProgressiveSamplingCurve = 2.0
	}

	return cfg, nil
}
func parseInt(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return n
}

func parseFloat(value string, fallback float32) float32 {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
	if err != nil {
		return fallback
	}
	return float32(n)
}

func parseFloat64(value string, fallback float64) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseBool(value string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseSaveAt(value string) map[int]struct{} {
	out := make(map[int]struct{})
	for _, part := range strings.Split(value, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 {
			continue
		}
		out[n] = struct{}{}
	}
	if len(out) == 0 {
		out[3000] = struct{}{}
	}
	return out
}
