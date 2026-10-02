package host

import (
	"github.com/neokapi/neokapi/core/contextop"
)

// personApplies is a person applying a change-set from a terminal, the actor
// `kapi apply` stamps when the environment names no agent.
var personApplies = changeActor{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Note: "applied with `kapi apply`"}
