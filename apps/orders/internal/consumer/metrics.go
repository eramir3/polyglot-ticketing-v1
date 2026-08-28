package consumer

import "polyglot-ticketing-v1/internal/observability"

func firstMetrics(metrics []*observability.Metrics) *observability.Metrics {
	if len(metrics) == 0 {
		return nil
	}
	return metrics[0]
}
