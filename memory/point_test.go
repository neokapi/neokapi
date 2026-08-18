package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPoint(t *testing.T) {
	tests := []struct {
		name                         string
		profile, channel, collection string
		want                         string
	}{
		{"a fully declared point", "neokapi", "cli", "neokapi-cli", "neokapi\x1fcli\x1fneokapi-cli"},
		{"a collection binding no channel", "", "", "app", "\x1f\x1fapp"},
		{"the project's default point", "", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NewPoint(tc.profile, tc.channel, tc.collection))
		})
	}
}

// TestPointDistance walks the containment ladder: the further apart two places
// are, the coarser the rung at which they stop agreeing.
func TestPointDistance(t *testing.T) {
	cli := NewPoint("neokapi", "cli", "neokapi-cli")
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"the same collection", cli, cli, 0},
		{"another collection on one channel", cli, NewPoint("neokapi", "cli", "neokapi-help"), 1},
		{"another channel of one product", cli, NewPoint("neokapi", "engine", "neokapi-engine"), 2},
		{"another product", cli, NewPoint("bowrain", "app", "bowrain-app"), 3},
		{"an answer bound to no location", cli, "", 3},
		{"two answers bound to no location", "", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, PointDistance(tc.a, tc.b))
			assert.Equal(t, tc.want, PointDistance(tc.b, tc.a), "distance is symmetric")
		})
	}
}

// TestNearerAnswer pins both halves of the resolution rule: nearest wins, and a
// genuine tie is settled by the answers alone.
func TestNearerAnswer(t *testing.T) {
	cli := NewPoint("neokapi", "cli", "neokapi-cli")
	engine := NewPoint("neokapi", "engine", "neokapi-engine")

	t.Run("the nearer approval wins", func(t *testing.T) {
		assert.True(t, NearerAnswer(cli, "Gjenbruk", engine, "Bruk om igjen", cli))
		assert.False(t, NearerAnswer(engine, "Bruk om igjen", cli, "Gjenbruk", cli))
	})

	t.Run("nearness outranks the text", func(t *testing.T) {
		// "Bruk om igjen" sorts first, and loses anyway: the tie-break only
		// speaks when the ladder has nothing to say.
		assert.True(t, NearerAnswer(cli, "Gjenbruk", engine, "Bruk om igjen", cli))
	})

	t.Run("two approvals at one point fall to the text", func(t *testing.T) {
		assert.True(t, NearerAnswer(cli, "Betegnelse", cli, "Navn", cli))
		assert.False(t, NearerAnswer(cli, "Navn", cli, "Betegnelse", cli))
	})

	t.Run("two equidistant points fall to the text", func(t *testing.T) {
		other := NewPoint("neokapi", "cli", "neokapi-help")
		at := NewPoint("neokapi", "cli", "neokapi-desktop")
		assert.Equal(t, PointDistance(cli, at), PointDistance(other, at))
		assert.True(t, NearerAnswer(cli, "Betegnelse", other, "Navn", at))
	})

	t.Run("a caller with no point in hand falls to the text", func(t *testing.T) {
		assert.True(t, NearerAnswer(cli, "Betegnelse", engine, "Navn", ""))
	})
}
