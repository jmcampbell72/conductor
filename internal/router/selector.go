package router

import "conductor/internal/config"

// Selector maps an Analysis and budget status to a model name.
type Selector struct {
	threshold float64
	simple    string
	complex   string
	writing   string
	qa        string
	economy   string
	override  string
}

func NewSelector(cfg config.RouteConfig) *Selector {
	return &Selector{
		threshold: cfg.ComplexityThreshold,
		simple:    cfg.Models.Simple,
		complex:   cfg.Models.Complex,
		writing:   cfg.Models.Writing,
		qa:        cfg.Models.QA,
		economy:   cfg.Models.Economy,
		override:  cfg.ModelOverride,
	}
}

// Select returns the model to use given a request analysis and budget state.
//
// Priority order:
//  1. model_override always wins when set.
//  2. Budget exceeded + economy model configured → economy (cost-conscious path).
//  3. Planning or writing task type → writing model (if configured).
//  4. QA task type → qa model (if configured).
//  5. Complexity score ≥ threshold → complex model.
//  6. Otherwise → simple model.
//
// Falls back to requested when required model slots are not configured.
func (s *Selector) Select(analysis Analysis, requested string, budgetExceeded bool) string {
	if s.override != "" {
		return s.override
	}

	if budgetExceeded && s.economy != "" {
		return s.economy
	}

	switch analysis.TaskType {
	case TaskPlanning, TaskWriting:
		if s.writing != "" {
			return s.writing
		}
	case TaskQA:
		if s.qa != "" {
			return s.qa
		}
	}

	if s.simple == "" || s.complex == "" {
		return requested
	}
	if analysis.Score >= s.threshold {
		return s.complex
	}
	return s.simple
}
