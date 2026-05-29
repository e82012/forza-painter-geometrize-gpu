package engine

import (
	"math"
	"math/rand"
	"sort"

	"forza-painter-geometrize-go/internal/model"
)

// CMAESBounds defines the search boundaries for candidate parameters
type CMAESBounds struct {
	MaxW   float32
	MaxH   float32
	MaxRad float32
}

// CMAESOptimizer implements the Covariance Matrix Adaptation Evolution Strategy for 5D (X, Y, RX, RY, Theta)
// operating in a normalized [0, 1]^5 search space.
type CMAESOptimizer struct {
	dim      int
	lambda   int
	mu       int
	weights  []float64
	mueff    float64
	
	// Paths
	pSigma []float64
	pC     []float64
	
	// Distribution state
	mean  []float64
	sigma float64
	C     [][]float64 // 5x5 covariance matrix
	
	// Eigen-decomposition cache
	B     [][]float64 // Eigenvectors
	diagD []float64   // Eigenvalues (diagonal of D, not squared)
	
	// Constants
	cc     float64
	cs     float64
	c1     float64
	cmu    float64
	damps  float64
	chiN   float64
	bounds CMAESBounds
	alpha  float32 // alpha of seed candidate, kept constant
	
	generation int
}

// NewCMAES initializes a new CMA-ES optimizer operating in a normalized [0, 1]^5 space.
func NewCMAES(seed model.Candidate, initSigma float64, lambda int, bounds CMAESBounds) *CMAESOptimizer {
	dim := 5
	if lambda <= 0 {
		lambda = 32 // default population size
	}
	mu := lambda / 2
	
	// Calculate weights
	weights := make([]float64, mu)
	sumW := 0.0
	sumW2 := 0.0
	for i := 0; i < mu; i++ {
		weights[i] = math.Log(float64(mu)+0.5) - math.Log(float64(i+1))
		sumW += weights[i]
	}
	for i := 0; i < mu; i++ {
		weights[i] /= sumW
		sumW2 += weights[i] * weights[i]
	}
	mueff := 1.0 / sumW2

	// Strategy parameter setting: Constants
	cs := (mueff + 2.0) / (float64(dim) + mueff + 5.0)
	cc := (4.0 + mueff/float64(dim)) / (float64(dim) + 4.0 + 2.0*mueff/float64(dim))
	c1 := 2.0 / (math.Pow(float64(dim)+1.3, 2) + mueff)
	cmu := math.Min(1.0-c1, 2.0*(mueff-2.0+1.0/mueff)/(math.Pow(float64(dim)+2.0, 2)+mueff))
	damps := 1.0 + 2.0*math.Max(0.0, math.Sqrt((mueff-1.0)/(float64(dim)+1.0))-1.0) + cs
	
	// Expectation of ||N(0,I)||
	chiN := math.Sqrt(float64(dim)) * (1.0 - 1.0/(4.0*float64(dim)) + 1.0/(21.0*math.Pow(float64(dim), 2)))

	// Initialize C to Identity matrix
	C := make([][]float64, dim)
	B := make([][]float64, dim)
	for i := 0; i < dim; i++ {
		C[i] = make([]float64, dim)
		C[i][i] = 1.0
		B[i] = make([]float64, dim)
		B[i][i] = 1.0
	}

	diagD := make([]float64, dim)
	for i := 0; i < dim; i++ {
		diagD[i] = 1.0
	}

	// Clamp bounds to prevent division by zero
	maxW := float64(bounds.MaxW)
	if maxW < 1.0 {
		maxW = 1.0
	}
	maxH := float64(bounds.MaxH)
	if maxH < 1.0 {
		maxH = 1.0
	}
	maxRad := float64(bounds.MaxRad)
	if maxRad <= 1.0 {
		maxRad = 2.0
	}

	// Map seed candidate to normalized space [0, 1]^5
	mean := []float64{
		float64(seed.X) / maxW,
		float64(seed.Y) / maxH,
		float64(seed.RX-1.0) / (maxRad - 1.0),
		float64(seed.RY-1.0) / (maxRad - 1.0),
		float64(seed.Theta) / 360.0,
	}

	return &CMAESOptimizer{
		dim:        dim,
		lambda:     lambda,
		mu:         mu,
		weights:    weights,
		mueff:      mueff,
		pSigma:     make([]float64, dim),
		pC:         make([]float64, dim),
		mean:       mean,
		sigma:      initSigma,
		C:          C,
		B:          B,
		diagD:      diagD,
		cc:         cc,
		cs:         cs,
		c1:         c1,
		cmu:        cmu,
		damps:      damps,
		chiN:       chiN,
		bounds:     bounds,
		alpha:      seed.A,
		generation: 0,
	}
}

// SamplePopulation generates lambda candidates in physical space from the normalized distribution.
// It returns candidates, along with their underlying z vectors and y vectors for later updates.
func (c *CMAESOptimizer) SamplePopulation(rng *rand.Rand) ([]model.Candidate, [][]float64, [][]float64) {
	candidates := make([]model.Candidate, c.lambda)
	zVectors := make([][]float64, c.lambda)
	yVectors := make([][]float64, c.lambda)

	maxW := float64(c.bounds.MaxW)
	if maxW < 1.0 {
		maxW = 1.0
	}
	maxH := float64(c.bounds.MaxH)
	if maxH < 1.0 {
		maxH = 1.0
	}
	maxRad := float64(c.bounds.MaxRad)
	if maxRad <= 1.0 {
		maxRad = 2.0
	}

	for k := 0; k < c.lambda; k++ {
		z := make([]float64, c.dim)
		for i := 0; i < c.dim; i++ {
			z[i] = rng.NormFloat64()
		}
		zVectors[k] = z

		// y = B * D * z
		y := make([]float64, c.dim)
		for i := 0; i < c.dim; i++ {
			sum := 0.0
			for j := 0; j < c.dim; j++ {
				sum += c.B[i][j] * c.diagD[j] * z[j]
			}
			y[i] = sum
		}
		yVectors[k] = y

		// x = mean + sigma * y
		x := make([]float64, c.dim)
		for i := 0; i < c.dim; i++ {
			x[i] = c.mean[i] + c.sigma*y[i]
			// Keep inside normalized boundaries [0, 1] using mirroring
			x[i] = mirrorBound(x[i], 0.0, 1.0)
		}

		// Map normalized coordinate back to physical candidate parameters
		candidates[k] = model.Candidate{
			X:     float32(x[0] * maxW),
			Y:     float32(x[1] * maxH),
			RX:    snapToValidRX(float32(1.0 + x[2]*(maxRad-1.0))),
			RY:    snapToValidRX(float32(1.0 + x[3]*(maxRad-1.0))),
			Theta: float32(x[4] * 360.0),
			A:     c.alpha,
		}
	}

	return candidates, zVectors, yVectors
}

// Update performs CMA-ES state adaptation based on population scores.
func (c *CMAESOptimizer) Update(candidates []model.Candidate, scores []float32, zVectors [][]float64, yVectors [][]float64) {
	// 1. Sort indices based on scores (lower score is better)
	type pair struct {
		idx   int
		score float32
	}
	pairs := make([]pair, c.lambda)
	for i := 0; i < c.lambda; i++ {
		pairs[i] = pair{idx: i, score: scores[i]}
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score < pairs[j].score
	})

	// 2. Compute y_mean = sum(w_i * y_i) and z_mean = sum(w_i * z_i)
	yMean := make([]float64, c.dim)
	zMean := make([]float64, c.dim)
	for i := 0; i < c.mu; i++ {
		w := c.weights[i]
		bestIdx := pairs[i].idx
		for d := 0; d < c.dim; d++ {
			yMean[d] += w * yVectors[bestIdx][d]
			zMean[d] += w * zVectors[bestIdx][d]
		}
	}

	// 3. Update mean: mean_new = mean_old + sigma * y_mean
	for d := 0; d < c.dim; d++ {
		c.mean[d] += c.sigma * yMean[d]
	}

	// 4. Update step size evolution path pSigma
	bzMean := make([]float64, c.dim)
	for i := 0; i < c.dim; i++ {
		sum := 0.0
		for j := 0; j < c.dim; j++ {
			sum += c.B[i][j] * zMean[j]
		}
		bzMean[i] = sum
	}

	constSigmaFactor := math.Sqrt(c.cs * (2.0 - c.cs) * c.mueff)
	pSigmaNormSq := 0.0
	for d := 0; d < c.dim; d++ {
		c.pSigma[d] = (1.0-c.cs)*c.pSigma[d] + constSigmaFactor*bzMean[d]
		pSigmaNormSq += c.pSigma[d] * c.pSigma[d]
	}
	pSigmaNorm := math.Sqrt(pSigmaNormSq)

	// 5. Update covariance evolution path pC
	hSigma := 0.0
	denomHSigma := 1.0 - math.Pow(1.0-c.cs, 2.0*float64(c.generation+1))
	if pSigmaNorm/math.Sqrt(denomHSigma) < (1.4+2.0/float64(c.dim+1))*c.chiN {
		hSigma = 1.0
	}

	constCFactor := hSigma * math.Sqrt(c.cc*(2.0-c.cc)*c.mueff)
	for d := 0; d < c.dim; d++ {
		c.pC[d] = (1.0-c.cc)*c.pC[d] + constCFactor*yMean[d]
	}

	// 6. Update Covariance Matrix C
	c1Factor := c.c1
	cmuFactor := c.cmu
	oldCFactor := 1.0 - c1Factor - cmuFactor

	rankMuUpdate := make([][]float64, c.dim)
	for i := 0; i < c.dim; i++ {
		rankMuUpdate[i] = make([]float64, c.dim)
	}
	for i := 0; i < c.mu; i++ {
		w := c.weights[i]
		bestIdx := pairs[i].idx
		y := yVectors[bestIdx]
		for r := 0; r < c.dim; r++ {
			for col := 0; col < c.dim; col++ {
				rankMuUpdate[r][col] += w * y[r] * y[col]
			}
		}
	}

	for r := 0; r < c.dim; r++ {
		for col := 0; col <= r; col++ {
			val := oldCFactor * c.C[r][col]
			val += c1Factor * (c.pC[r]*c.pC[col] + (1.0-hSigma)*c.cc*(2.0-c.cc)*c.C[r][col])
			val += cmuFactor * rankMuUpdate[r][col]
			c.C[r][col] = val
			if col != r {
				c.C[col][r] = val // symmetric
			}
		}
	}

	// 7. Update step size sigma
	c.sigma *= math.Exp((c.cs / c.damps) * (pSigmaNorm/c.chiN - 1.0))
	c.sigma = math.Max(1e-5, math.Min(1.0, c.sigma)) // Limit step size to unit bounds

	// 8. Perform Eigen-decomposition of C to find B and diagD
	c.eigenDecomposition()
	c.generation++
}

func (c *CMAESOptimizer) eigenDecomposition() {
	A := make([][]float64, c.dim)
	for i := 0; i < c.dim; i++ {
		A[i] = make([]float64, c.dim)
		copy(A[i], c.C[i])
	}

	for i := 0; i < c.dim; i++ {
		for j := 0; j < c.dim; j++ {
			if i == j {
				c.B[i][j] = 1.0
			} else {
				c.B[i][j] = 0.0
			}
		}
	}

	maxIter := 50
	for iter := 0; iter < maxIter; iter++ {
		row, col := 0, 1
		maxVal := 0.0
		for i := 0; i < c.dim; i++ {
			for j := i + 1; j < c.dim; j++ {
				absVal := math.Abs(A[i][j])
				if absVal > maxVal {
					maxVal = absVal
					row, col = i, j
				}
			}
		}

		if maxVal < 1e-12 {
			break
		}

		phi := 0.5 * math.Atan2(2.0*A[row][col], A[row][row]-A[col][col])
		cosP := math.Cos(phi)
		sinP := math.Sin(phi)

		aRowRow := A[row][row]
		aColCol := A[col][col]
		aRowCol := A[row][col]

		A[row][row] = cosP*cosP*aRowRow + 2.0*sinP*cosP*aRowCol + sinP*sinP*aColCol
		A[col][col] = sinP*sinP*aRowRow - 2.0*sinP*cosP*aRowCol + cosP*cosP*aColCol
		A[row][col] = 0.0
		A[col][row] = 0.0

		for i := 0; i < c.dim; i++ {
			if i != row && i != col {
				aIRow := A[i][row]
				aICol := A[i][col]
				A[i][row] = cosP*aIRow + sinP*aICol
				A[row][i] = A[i][row]
				A[i][col] = -sinP*aIRow + cosP*aICol
				A[col][i] = A[i][col]
			}
		}

		for i := 0; i < c.dim; i++ {
			bIRow := c.B[i][row]
			bICol := c.B[i][col]
			c.B[i][row] = cosP*bIRow + sinP*bICol
			c.B[i][col] = -sinP*bIRow + cosP*bICol
		}
	}

	for i := 0; i < c.dim; i++ {
		eigenVal := A[i][i]
		if eigenVal < 1e-10 {
			eigenVal = 1e-10
		}
		c.diagD[i] = math.Sqrt(eigenVal)
	}
}

// Best returns the candidate corresponding to the current mean in physical space.
func (c *CMAESOptimizer) Best() model.Candidate {
	maxW := float64(c.bounds.MaxW)
	if maxW < 1.0 {
		maxW = 1.0
	}
	maxH := float64(c.bounds.MaxH)
	if maxH < 1.0 {
		maxH = 1.0
	}
	maxRad := float64(c.bounds.MaxRad)
	if maxRad <= 1.0 {
		maxRad = 2.0
	}

	x0 := mirrorBound(c.mean[0], 0.0, 1.0)
	x1 := mirrorBound(c.mean[1], 0.0, 1.0)
	x2 := mirrorBound(c.mean[2], 0.0, 1.0)
	x3 := mirrorBound(c.mean[3], 0.0, 1.0)
	x4 := mirrorBound(c.mean[4], 0.0, 1.0)

	return model.Candidate{
		X:     float32(x0 * maxW),
		Y:     float32(x1 * maxH),
		RX:    snapToValidRX(float32(1.0 + x2*(maxRad-1.0))),
		RY:    snapToValidRX(float32(1.0 + x3*(maxRad-1.0))),
		Theta: float32(x4 * 360.0),
		A:     c.alpha,
	}
}

// mirrorBound reflects val back into boundary if it overflows/underflows.
func mirrorBound(val, minVal, maxVal float64) float64 {
	if minVal >= maxVal {
		return minVal
	}
	for {
		if val < minVal {
			val = minVal + (minVal - val)
		} else if val > maxVal {
			val = maxVal - (val - maxVal)
		} else {
			break
		}
	}
	if val < minVal {
		val = minVal
	}
	if val > maxVal {
		val = maxVal
	}
	return val
}
