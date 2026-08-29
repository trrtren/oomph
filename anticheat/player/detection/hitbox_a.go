package detection

import (
	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/ethaniccc/float32-cube/cube"
	"github.com/oomph-ac/oomph/anticheat/game"
	"github.com/oomph-ac/oomph/anticheat/player"
	"github.com/oomph-ac/oomph/anticheat/utils"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type HitboxA struct {
	mPlayer  *player.Player
	metadata *player.DetectionMetadata
}

func New_HitboxA(p *player.Player) *HitboxA {
	return &HitboxA{
		mPlayer: p,
		metadata: &player.DetectionMetadata{
			FailBuffer:    6,
			MaxBuffer:     6,
			MaxViolations: 10,
			//TrustDuration: 60 * player.TicksPerSecond,
		},
	}
}

func (*HitboxA) Type() string {
	return TypeHitbox
}

func (*HitboxA) SubType() string {
	return "A"
}

func (*HitboxA) Description() string {
	return "Checks if the player is using a hitbox modification greater than the one sent by the server."
}

func (*HitboxA) Punishable() bool {
	return true
}

func (d *HitboxA) Metadata() *player.DetectionMetadata {
	return d.metadata
}

func (d *HitboxA) Detect(pk packet.Packet) {
	if !d.mPlayer.Opts().Combat.EnableClientEntityTracking {
		return
	}
	switch pk := pk.(type) {
	case *packet.Interact:
		interactPos, ok := pk.Position.Value()
		if pk.ActionType != packet.InteractActionMouseOverEntity || !ok {
			//d.mPlayer.Message("%d %v", pk.ActionType, pk.Position)
			return
		}
		
		// skip check if player is in liquid
		stateBB := d.mPlayer.Movement().BoundingBox()
		for result := range utils.NearbyBlocks(stateBB.Grow(1), false, true, d.mPlayer.World()) {
			if _, isLiquid := result.Block.(world.Liquid); isLiquid {
				blockBB := cube.Box(0, 0, 0, 1, 1, 1).Translate(result.Position.Vec3())
				if stateBB.IntersectsWith(blockBB) {
					return
				}
			}
		}
		
		e := d.mPlayer.ClientEntityTracker().FindEntity(pk.TargetEntityRuntimeID)
		if e == nil || !e.IsPlayer || e.TicksSinceTeleport <= 10 {
			return
		}
		h1 := e.Box(e.PrevPosition).Grow(0.1)
		h2 := e.Box(e.Position).Grow(0.1)
		dist := math32.Min(
			interactPos.Sub(game.ClosestPointToBBox(interactPos, h1)).Len(),
			interactPos.Sub(game.ClosestPointToBBox(interactPos, h2)).Len(),
		)
		if dist > 0.004 {
			d.mPlayer.FailDetection(d, "amt", game.Round32(0.6+(dist*2), 3))
		} else {
			d.mPlayer.PassDetection(d, d.metadata.Buffer)
		}
	}
}
