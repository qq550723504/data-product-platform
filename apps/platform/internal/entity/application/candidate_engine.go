package application

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/matching"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

// UseCandidateGenerator enables an optional probabilistic candidate engine for
// subsequent match jobs. Canonical Entity/EntityMapping ownership remains in
// MatchService; the generator can only propose candidates through the SPI.
func (s *MatchService) UseCandidateGenerator(generator resolution.CandidateGenerator) {
	if generator == nil {
		s.engine = matching.NewEngine(s.entityRepo)
		return
	}
	s.engine = matching.NewEngineWithCandidateGenerator(s.entityRepo, generator)
}
