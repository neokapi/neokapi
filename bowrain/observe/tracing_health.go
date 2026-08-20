package observe

// Health probes are traffic, not signal.
//
// The load balancer probes the HTTP health route and the gRPC health method
// every 15 seconds (bowrain-infra, modules/alb/targets.tf). A probe's duration
// is already an ALB target-health metric, so a transaction per probe restates
// what is measured elsewhere and drags every aggregate toward the cheapest
// endpoint on the server.
//
// It also never arrived. No health transaction has ever appeared in Sentry,
// although the probe returns 200 through this middleware with the traces sample
// rate at 1 — so they were produced and discarded before storage rather than
// stored and ignored. Not producing them reaches the same end without the work.
//
// The gRPC method is listed even though no health service is registered today.
// The ALB matcher accepts UNIMPLEMENTED for exactly that reason, and grpc-go
// answers an unregistered method without running interceptors, so the entry is
// inert as things stand. The probe is continuous and registering the service is
// an ordinary thing to do later; naming the method here means doing so cannot
// quietly start producing a transaction every fifteen seconds.
const (
	healthRouteHTTP  = "/api/v1/health"
	healthMethodGRPC = "/grpc.health.v1.Health/Check"
)

// isHealthProbe reports whether a matched Echo route or gRPC method is one the
// load balancer polls for liveness.
//
// Matched route, never the concrete URL: the comparison has to survive the same
// parameterization the transaction name relies on, and a caller that reaches
// the health route by some other path is still a health check.
func isHealthProbe(route string) bool {
	return route == healthRouteHTTP || route == healthMethodGRPC
}
