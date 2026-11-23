package project

import "github.com/neokapi/neokapi/core/redaction"

// A redaction vault kapi writes into a project's vault directory is written in
// step with the vault's mark, as every other file there is.
func init() {
	redaction.WriteVaultFile = WriteStateFile
}
