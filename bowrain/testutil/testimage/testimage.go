// Package testimage names every container image the bowrain suites start, so
// the tests and the CI step that fetches them ahead of the run read the same
// list. A pre-pull that names an image the tests do not start fetches nothing
// they need, and the test pulls its own image at the moment it runs, which is
// the failure the pre-pull exists to move.
//
// Each tag is a released version rather than `latest`: an image that changes
// under the suite makes a run irreproducible, and a pre-pull of `latest` warms
// the cache with whatever was published that morning.
package testimage

// The images, one constant per service the suites stand up.
const (
	// Redis backs the event bus tests (bowrain/event).
	Redis = "redis:7-alpine"
	// Postgres backs every store suite through testutil/pgtest, on a runner
	// that offers no BOWRAIN_TEST_POSTGRES_URL of its own.
	Postgres = "postgres:16-alpine"
	// MinIO serves the S3 API the blob store tests run against
	// (bowrain/storage/s3blob).
	MinIO = "minio/minio:RELEASE.2025-09-07T16-13-09Z"
	// ElasticMQ serves the SQS API the job queue tests run against
	// (bowrain/jobs).
	ElasticMQ = "softwaremill/elasticmq-native:1.7.1"
)

// All returns every image the suites start, in a stable order.
func All() []string {
	return []string{Redis, Postgres, MinIO, ElasticMQ}
}
