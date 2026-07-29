package router

import "conductor/internal/config"

// Selector maps a complexity score to a model name.
type Selector struct {
	threshold float64
	simple    string
	complex   string
	override  string
}

func NewSelector(cfg config.RouteConfig) *Selector {
	return &Selector{
		threshold: cfg.ComplexityThreshold,
		simple:    cfg.Models.Simple,
		complex:   cfg.Models.Complex,
		override:  cfg.ModelOverride,
	}
}

// Select returns the model to use for the given score.
//   - If the route has a model_override it always wins.
//   - If no simple/complex models are configured, requested is returned unchanged.
//   - Otherwise, score >= threshold → complex model; below → simple model.
func (s *Selector) Select(score float64, requested string) string {
	if s.override != "" {
		return s.override
	}
	if s.simple == "" || s.complex == "" {
		return requested
	}
	if score >= s.threshold {
		return s.complex
	}
	return s.simple
}
