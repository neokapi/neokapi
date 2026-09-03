package project

import "fmt"

// DeclareChannel adds a channel to a profile, creating the profile if it does
// not exist yet. A channel is a surface of a product, addressed profile/channel,
// so declaring one names both. It is the recipe write behind "add a channel".
//
// It refuses a channel already declared on the profile, and a profile or channel
// that is not a slug, with the same rule every other recipe write uses.
func (p *KapiProject) DeclareChannel(profile, channel string) error {
	if !slugPattern.MatchString(profile) {
		return fmt.Errorf("recipe: profile %q must be %s", profile, slugRule)
	}
	if !slugPattern.MatchString(channel) {
		return fmt.Errorf("recipe: channel %q must be %s", channel, slugRule)
	}
	if p.declaresChannelRef(profile, channel) {
		return fmt.Errorf("recipe: %s/%s is already declared", profile, channel)
	}
	p.declareChannel(profile, channel)
	return nil
}

// RenameChannel renames a profile-declared channel and moves every collection
// that named it to the new id.
//
// The two happen together for the same reason setCollectionField declares a
// channel when a collection binds to it: a collection whose channel names no
// declared point does not load, so leaving the collections behind would produce
// a recipe that does not load. A channel that only exists because a collection
// references it is derived, not declared, and is refused here.
func (p *KapiProject) RenameChannel(profile, oldChannel, newChannel string) error {
	if !slugPattern.MatchString(newChannel) {
		return fmt.Errorf("recipe: channel %q must be %s", newChannel, slugRule)
	}
	if oldChannel == newChannel {
		return nil
	}
	if !p.declaresChannelRef(profile, oldChannel) {
		return fmt.Errorf("recipe: %s/%s is not a declared channel", profile, oldChannel)
	}
	if p.declaresChannelRef(profile, newChannel) {
		return fmt.Errorf("recipe: %s/%s already exists", profile, newChannel)
	}

	prof := p.Profiles[profile]
	for i := range prof.Channels {
		if prof.Channels[i].ID == oldChannel {
			prof.Channels[i].ID = newChannel
		}
	}
	p.Profiles[profile] = prof

	oldRef := profile + "/" + oldChannel
	newRef := profile + "/" + newChannel
	for i := range p.Collections {
		if p.Collections[i].Channel == oldRef {
			p.Collections[i].Channel = newRef
		}
	}
	return nil
}
