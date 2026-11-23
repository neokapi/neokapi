//go:build !unix

package pluginhost

// isCrossDevice reports a rename refused across volumes. On Windows the
// error is ERROR_NOT_SAME_DEVICE, and elsewhere there is no such refusal;
// any rename error there is taken as one, so the copy runs.
func isCrossDevice(error) bool { return true }
