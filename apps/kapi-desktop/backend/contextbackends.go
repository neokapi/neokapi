package backend

// The context backends beyond the ones host opens itself register on import,
// as they do in the kapi binary, so a project sharing its context through one
// shows its sync state in the desktop too.
import _ "github.com/neokapi/neokapi/host/s3remote"
