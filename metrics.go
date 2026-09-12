package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	deliveryResults = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_deliveries_total",
			Help: "Total number of webhook delivery attempts by result.",
		},
		[]string{"result"},
	)

	deliveryDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "webhook_delivery_duration_seconds",
			Help: "Time spent delivering a webhook.",
		},
	)

	deliveryAttempts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webhook_delivery_attempts_total",
			Help: "Total number of webhook delivery attempts.",
		},
	)

	queueSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "webhook_queue_size",
			Help: "Current number of deliveries waiting in the in-memory queue.",
		},
	)
)

func initMetrics() {
	prometheus.MustRegister(deliveryResults)
	prometheus.MustRegister(deliveryDuration)
	prometheus.MustRegister(deliveryAttempts)
	prometheus.MustRegister(queueSize)
}
