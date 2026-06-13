// Package nsr implements the NetWorker collection layer: the background collection
// loop, the immutable snapshot store, the per-domain resource collectors, and the
// two export paths (Prometheus unchecked collector and OTLP observable gauges) that
// both read the same snapshot. See docs/adr/ for the load-bearing decisions.
package nsr
