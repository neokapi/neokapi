package observe

// Health probes are traffic, not signal.
//
// The load balancer probes the HTTP health route and the gRPC health method
// every 15 seconds (bowrain-infra, modules/alb/targets.tf). At one task per
// service that is about 5,760 probes a day against roughly 7,000 real requests,
// so tracing them would spend nearly half the transaction budget describing a
// request whose duration is already an ALB target-health metric — and would
// drag every aggregate toward the cheapest endpoint in the process.
//
// The gRPC method is listed even though no health service is registered today.
// The ALB matcher accepts UNIMPLEMENTED for exactly that reason, and grpc-go
// answers an unregistered method without running interceptors, so the entry is
// inert as things stand. The probe is continuous and registering the service is
// an ordinary thing to do later; naming the method here means doing so does not
// quietly start producing 5,760 transactions a day that nobody asked for.
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
