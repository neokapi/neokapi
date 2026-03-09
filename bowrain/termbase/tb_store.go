package termbase

import fw "github.com/gokapi/gokapi/core/termbase"

// TBStore extends TermBase with the full method set needed by the
// bowrain server. The core TermBase interface already includes Search
// and Concepts, so this is currently a type alias for documentation
// clarity and future extension.
type TBStore = fw.TermBase
