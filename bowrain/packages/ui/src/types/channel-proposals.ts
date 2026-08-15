/**
 * Channel-slug equivalence proposals: the workspace's record that two projects
 * spell one channel differently, mirroring `GET /:ws/context/channel-proposals`.
 *
 * A workspace owns equivalence, never resolution. A project resolves its own
 * coordinates from its own recipe, offline, so accepting a proposal records
 * agreement between people and rewrites nobody's slug.
 */

/** A proposal's standing: raised, agreed with, or judged apart. */
export type ChannelProposalStatus = "proposed" | "accepted" | "dismissed";

export interface ChannelAliasProposal {
  /** The product-axis value both slugs sit under. */
  profile?: string;
  /** The slug that arrived. */
  proposed_channel: string;
  /** The slug the workspace already held. */
  existing_channel: string;
  /** Why the two look alike, in the framework's own words, so a reviewer
   *  judges the observation rather than trusting it. */
  evidence?: string;
  /** Where the push that raised it landed. */
  project_id?: string;
  collection?: string;
  status?: ChannelProposalStatus;
  /** Who settled it, and when — distinct from updated_at, which every
   *  re-sighting of the same fragmentation refreshes. */
  judged_by?: string;
  judged_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ChannelAliasProposalsResponse {
  proposals: ChannelAliasProposal[];
}

/** The verdict a reviewer sends: the proposal's key plus the status chosen. */
export interface ChannelAliasJudgement {
  profile?: string;
  proposed_channel: string;
  existing_channel: string;
  status: "accepted" | "dismissed";
}
