package workflow

import (
	"fmt"
	"strings"
)

var supportedLoopModes = map[string]bool{
	"count":            true,
	"until_success":    true,
	"until_condition":  true,
	"while_condition":  true,
	"for_each":         true,
	"interval":         true,
	"retry_backoff":    true,
	"infinite_guarded": true,
}

func validateLoopNodeConfig(loop *LoopNodeConfig) error {
	if loop == nil {
		return nil
	}
	mode := strings.TrimSpace(loop.Mode)
	if mode == "" {
		return fmt.Errorf("mode is required")
	}
	if !supportedLoopModes[mode] {
		return fmt.Errorf("unsupported mode %q", mode)
	}
	if loop.MaxIterations < 0 || loop.DelayMs < 0 || loop.TimeoutMs < 0 {
		return fmt.Errorf("numeric guards cannot be negative")
	}
	if loop.OnError != "" {
		switch loop.OnError {
		case "stop", "continue", "retry", "skip":
		default:
			return fmt.Errorf("unsupported on_error %q", loop.OnError)
		}
	}
	if requiresIterationGuard(mode) && loop.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations is required for mode %s", mode)
	}
	if requiresDelayGuard(mode) && loop.DelayMs <= 0 {
		return fmt.Errorf("delay_ms is required for mode %s", mode)
	}
	if (mode == "until_condition" || mode == "while_condition") && strings.TrimSpace(loop.Condition) == "" {
		return fmt.Errorf("condition is required for mode %s", mode)
	}
	if mode == "for_each" && strings.TrimSpace(loop.ItemsSource) == "" {
		return fmt.Errorf("items_source is required for for_each mode")
	}
	if mode == "retry_backoff" {
		if loop.Retry == nil {
			return fmt.Errorf("retry config is required for retry_backoff mode")
		}
		if loop.Retry.MaxAttempts <= 0 {
			return fmt.Errorf("retry.max_attempts must be greater than zero")
		}
		if loop.Retry.InitialDelayMs < 0 || loop.Retry.MaxDelayMs < 0 || loop.Retry.Multiplier < 0 {
			return fmt.Errorf("retry numeric values cannot be negative")
		}
	}
	if mode == "infinite_guarded" && loop.TimeoutMs <= 0 && loop.MaxIterations <= 0 {
		return fmt.Errorf("infinite_guarded requires timeout_ms or max_iterations")
	}
	return nil
}

func requiresIterationGuard(mode string) bool {
	switch mode {
	case "count", "until_success", "until_condition", "while_condition", "interval":
		return true
	default:
		return false
	}
}

func requiresDelayGuard(mode string) bool {
	switch mode {
	case "interval", "infinite_guarded":
		return true
	default:
		return false
	}
}
