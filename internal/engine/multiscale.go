package engine

import (
	"fmt"
	"math/rand"
	"time"

	"forza-painter-geometrize-go/internal/config"
	"forza-painter-geometrize-go/internal/gpu"
	"forza-painter-geometrize-go/internal/imageutil"
	"forza-painter-geometrize-go/internal/model"
	"forza-painter-geometrize-go/internal/output"
	"forza-painter-geometrize-go/internal/render"
)

// Run is the main entry point which dispatches to SinglePass or MultiScale
func Run(opts Options) error {
	if opts.ImagePath == "" {
		return fmt.Errorf("image path is required")
	}
	if opts.WorkspaceRoot == "" {
		opts.WorkspaceRoot = "."
	}

	settingsPath, err := config.ResolveSettingsPath(opts.WorkspaceRoot, opts.SettingsPath, opts.Profile)
	if err != nil {
		return err
	}
	cfg, err := config.ParseSettings(settingsPath)
	if err != nil {
		return err
	}

	prepared, err := imageutil.LoadAndPrepare(opts.ImagePath, cfg.MaxResolution)
	if err != nil {
		return err
	}

	if opts.EdgeWeight >= 0 {
		cfg.EdgeWeight = opts.EdgeWeight
	}

	if opts.MultiScale {
		cfg.MultiScale = true
	}
	if opts.SavePassPreviews {
		cfg.SavePassPreviews = true
	}

	if cfg.MultiScale {
		return runMultiScale(opts, cfg, prepared)
	}
	return runSinglePass(opts, cfg, prepared)
}

func runMultiScale(opts Options, cfg model.Settings, prepared *imageutil.PreparedImage) error {
	passes := DefaultPassConfigs(cfg.StopAt)
	var allShapes []model.Shape

	globalCurrent := make([]float32, len(prepared.Current))
	copy(globalCurrent, prepared.Current)

	shapeOffset := 0
	for passIdx, passCfg := range passes {
		fmt.Printf("\n=== [MultiScale] Pass %d/%d: %s ===\n", passIdx+1, len(passes), passCfg.Name)

		scaledTarget, scaleW, scaleH := ScaleImageFloat32(prepared.Target, prepared.Width, prepared.Height, passCfg.ScaleSize)
		scaledCurrent, _, _ := ScaleImageFloat32(globalCurrent, prepared.Width, prepared.Height, passCfg.ScaleSize)
		scaledMask := ScaleMask(prepared.OpaqueMask, prepared.Width, prepared.Height, scaleW, scaleH)

		passPrepared := &imageutil.PreparedImage{
			Width:           scaleW,
			Height:          scaleH,
			Target:          scaledTarget,
			Current:         scaledCurrent,
			OpaqueMask:      scaledMask,
			HasTransparency: prepared.HasTransparency,
			BackgroundRGBA:  prepared.BackgroundRGBA,
		}

		passCfgCopy := passCfg
		passShapes, err := runPass(opts, cfg, passPrepared, &passCfgCopy, shapeOffset)
		if err != nil {
			return err
		}

		var passScaledShapes []model.Shape
		for _, s := range passShapes {
			if s.Type == 1 { // Skip background
				continue
			}
			rescaled := RescaleShapeCoords(s, scaleW, scaleH, prepared.Width, prepared.Height)
			passScaledShapes = append(passScaledShapes, rescaled)
		}
		allShapes = append(allShapes, passScaledShapes...)
		shapeOffset += len(passScaledShapes)

		// Render newly added shapes to globalCurrent
		fullEval, err := gpu.NewEvaluator(prepared.Target, globalCurrent, prepared.OpaqueMask, prepared.Width, prepared.Height, 1)
		if err == nil {
			for _, s := range passScaledShapes {
				cand := model.Candidate{
					X:     float32(s.Data[0]),
					Y:     float32(s.Data[1]),
					RX:    float32(s.Data[2]),
					RY:    float32(s.Data[3]),
					Theta: float32(s.Data[4]),
					R:     float32(s.Color[0]) / 255.0,
					G:     float32(s.Color[1]) / 255.0,
					B:     float32(s.Color[2]) / 255.0,
					A:     float32(s.Color[3]) / 255.0,
				}
				fullEval.Apply(cand)
			}
			fullEval.ReadCurrent(globalCurrent)
			fullEval.Close()
		} else {
			return fmt.Errorf("failed to create full res evaluator: %v", err)
		}

		if cfg.SavePassPreviews {
			outPath := fmt.Sprintf("%s.pass_%d.png", resolveOutputBase(opts), passIdx+1)
			render.SavePNG(outPath, globalCurrent, prepared.Width, prepared.Height)
		}
	}

	finalShapes := append([]model.Shape{backgroundShape(prepared, 0)}, allShapes...)
	if err := output.SaveGeometry(output.BuildFinalOutputPath(resolveOutputBase(opts)), finalShapes); err != nil {
		return err
	}

	if opts.PreviewPath != "" {
		if err := render.SavePNG(opts.PreviewPath, globalCurrent, prepared.Width, prepared.Height); err != nil {
			return err
		}
	}

	return nil
}

func runPass(opts Options, cfg model.Settings, prepared *imageutil.PreparedImage, passCfg *model.PassConfig, shapeOffset int) ([]model.Shape, error) {
	maxBatch := cfg.RandomSamples
	if cfg.MutatedSamples > maxBatch {
		maxBatch = cfg.MutatedSamples
	}
	evaluator, err := gpu.NewEvaluator(prepared.Target, prepared.Current, prepared.OpaqueMask, prepared.Width, prepared.Height, maxBatch)
	if err != nil {
		return nil, err
	}
	defer evaluator.Close()

	if passCfg.EdgeWeight > 0 {
		edgeMap := ComputeEdgeMap(prepared.Target, prepared.Width, prepared.Height)
		if err := evaluator.SetEdgeMap(edgeMap, float32(passCfg.EdgeWeight)); err != nil {
			return nil, err
		}
		fmt.Printf("[EdgeGuided] Edge map computed and uploaded to GPU, weight=%.1f\n", passCfg.EdgeWeight)
	}

	evaluator.UseWorkGroupEval = cfg.UseWorkGroupEval

	rng := rand.New(rand.NewSource(seedValue(opts.Seed + int64(shapeOffset))))
	currentError, opaquePixels := computeTotalError(prepared.Target, prepared.Current, prepared.OpaqueMask)
	denom := float64(maxInt(1, opaquePixels*4))

	shapes := []model.Shape{backgroundShape(prepared, normalizeScore(currentError, denom))}

	lambda := 32
	generations := cfg.MutatedSamples / lambda

	initialGrid, gw, gh, err := evaluator.ErrorGrid()
	if err != nil {
		return nil, err
	}
	sampler := newErrorSampler(initialGrid, gw, gh, prepared.Width, prepared.Height)
	var pendingGrid gpu.GridTicket

	acceptedShapes := 0
	consecutiveNoImprove := 0

	maxDim := float32(prepared.Width)
	if float32(prepared.Height) > maxDim {
		maxDim = float32(prepared.Height)
	}
	minRad := float32(passCfg.MinRadius) * maxDim
	maxRad := float32(passCfg.MaxRadius) * maxDim
	if minRad < 1 { minRad = 1 }
	if maxRad < minRad { maxRad = minRad + 1 }

	for acceptedShapes < passCfg.ShapeCount {
		stepStart := time.Now()
		progress := float32(acceptedShapes) / float32(passCfg.ShapeCount)
		evaluator.SampleStep = scoringSampleStep(cfg, progress)

		// Keep edge weight constant at the user-specified value to ensure strict edge guided detail alignment.
		if passCfg.EdgeWeight > 0 {
			evaluator.SetEdgeWeight(float32(passCfg.EdgeWeight))
		}

		randomCands := randomCandidates(rng, prepared, cfg.RandomSamples, cfg.ForceOpaqueShapes, sampler, minRad, maxRad)

		best, bestScore, err := submitAndPickBest(evaluator, randomCands, acceptedShapes+shapeOffset)
		if err != nil {
			return nil, err
		}

		if generations > 0 && bestScore < 0 {
			initialSigma := progressiveInitialSigma(progress)
			bounds := CMAESBounds{
				MaxW:   float32(prepared.Width),
				MaxH:   float32(prepared.Height),
				MaxRad: maxRad,
			}
			cma := NewCMAES(best, initialSigma, lambda, bounds)

			for gen := 0; gen < generations; gen++ {
				population, zVecs, yVecs := cma.SamplePopulation(rng)
				t, err := evaluator.SubmitEval(population)
				if err != nil {
					return nil, err
				}
				results, err := evaluator.WaitEval(t)
				if err != nil {
					return nil, err
				}

				genBestIdx := 0
				genBestScore := results[0].Score
				for i := 1; i < len(results); i++ {
					if results[i].Score < genBestScore {
						genBestScore = results[i].Score
						genBestIdx = i
					}
				}

				if genBestScore < bestScore {
					bestScore = genBestScore
					best = population[genBestIdx]
					best.R = results[genBestIdx].R
					best.G = results[genBestIdx].G
					best.B = results[genBestIdx].B
				}

				scores := make([]float32, len(results))
				for i := 0; i < len(results); i++ {
					scores[i] = results[i].Score
					population[i].R = results[i].R
					population[i].G = results[i].G
					population[i].B = results[i].B
				}

				cma.Update(population, scores, zVecs, yVecs)
			}
		}

		if bestScore >= minImproveDelta {
			consecutiveNoImprove++
			if consecutiveNoImprove >= maxNoImproveRetries {
				fmt.Printf("[%s] Stopped early: reached max retries without improvement\n", passCfg.Name)
				break
			}
			continue
		}
		consecutiveNoImprove = 0

		final := quantizeCandidate(best, prepared.Width, prepared.Height, cfg.ForceOpaqueShapes)
		if err := evaluator.SubmitApply(final); err != nil {
			return nil, err
		}
		
		currentError += float64(bestScore)
		if currentError < 0 { currentError = 0 }
		
		shapes = append(shapes, toShape(final, normalizeScore(currentError, denom)))
		acceptedShapes++
		
		if acceptedShapes%50 == 0 {
			fmt.Printf("[%s] %d/%d shapes added\n", passCfg.Name, acceptedShapes, passCfg.ShapeCount)
		}

		globalShapeCount := acceptedShapes + shapeOffset
		if shouldSavePreview(globalShapeCount, cfg) {
			if err := savePreviewSnapshot(evaluator, opts, prepared.Width, prepared.Height, globalShapeCount); err != nil {
				return nil, err
			}
			if opts.PreviewPath != "" {
				fmt.Printf("[%d/%d] Saved preview snapshot\n", globalShapeCount, cfg.StopAt)
			}
		}

		if pendingGrid.Valid() {
			grid, gridW, gridH, gErr := evaluator.WaitErrorGrid(pendingGrid)
			if gErr != nil {
				return nil, gErr
			}
			sampler = newErrorSampler(grid, gridW, gridH, prepared.Width, prepared.Height)
			pendingGrid = gpu.GridTicket{}
		}

		newTicket, gErr := evaluator.SubmitErrorGrid()
		if gErr != nil {
			return nil, gErr
		}
		pendingGrid = newTicket

		fmt.Printf("[%d/%d] Step completed in %s\n", globalShapeCount, cfg.StopAt, time.Since(stepStart).Round(time.Millisecond))
	}

	if pendingGrid.Valid() {
		if _, _, _, err := evaluator.WaitErrorGrid(pendingGrid); err != nil {
			return nil, err
		}
	}

	return shapes, nil
}
