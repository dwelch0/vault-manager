package utils

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	reconcileSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qontract_reconcile_execution_counter",
			Help: "Increment by one for each successful reconcile. Used to alert on 'stuck' instance reconciles",
		},
		[]string{
			"address",
			"integration",
		},
	)
	lastReconcileSuccessGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qontract_reconcile_last_run_status",
			Help: `Whether or not last reconcile for a specific vault instance was successful. ` +
				`A reconcile is successful if no errors occur. 0 = success. 1 = failure.`,
		},
		[]string{
			"address",
			"integration",
		},
	)
	executionDurationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qontract_reconcile_last_run_seconds",
			Help: "Execution duration of this job (reconciling all vault instances) in seconds.",
		},
		[]string{
			"address",
			"integration",
		},
	)
)

// register custom metrics at package import
func init() {
	prometheus.MustRegister(reconcileSuccessCounter)
	prometheus.MustRegister(lastReconcileSuccessGauge)
	prometheus.MustRegister(executionDurationGauge)
}

func RecordMetrics(instance string, status int, duration time.Duration) {
	const INTEGRATION = "vault-manager"

	lastReconcileSuccessGauge.With(
		prometheus.Labels{
			"address":     instance,
			"integration": INTEGRATION,
		}).Set(float64(status))

	// only inc counter metric for successful reconciles
	if status == 0 {
		reconcileSuccessCounter.With(
			prometheus.Labels{
				"address":     instance,
				"integration": INTEGRATION,
			}).Inc()
	}

	executionDurationGauge.With(
		prometheus.Labels{
			"address":     instance,
			"integration": INTEGRATION,
		}).Set(duration.Seconds())
}