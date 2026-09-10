package detection

import (
	"math"
	"sort"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	df_cube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/ethaniccc/float32-cube/cube"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/oomph-ac/oomph/anticheat/player"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	rotationSamples = 150
	epsilon         = 1e-5
	maxPingMs       = 350
)

type AimB struct {
	mPlayer  *player.Player
	metadata *player.DetectionMetadata

	rotationSamples []float32
	lastYaw         float32
	initialized     bool
}

func New_AimB(p *player.Player) *AimB {
	return &AimB{
		mPlayer: p,
		metadata: &player.DetectionMetadata{
			FailBuffer: 1,
			MaxBuffer:  1,

			MaxViolations: 5,
		},
		rotationSamples: make([]float32, 0, rotationSamples),
	}
}

func (*AimB) Type() string {
	return TypeAim
}

func (*AimB) SubType() string {
	return "B"
}

func (*AimB) Description() string {
	return "Checks for consistent rotation slopes indicating aim assist patterns."
}

func (*AimB) Punishable() bool {
	return true
}

func (d *AimB) Metadata() *player.DetectionMetadata {
	return d.metadata
}

func (d *AimB) Detect(pk packet.Packet) {
	input, ok := pk.(*packet.PlayerAuthInput)
	if !ok {
		return
	}

	// this checks for mouse input mode
	if input.InputMode != packet.InputModeMouse {
		return
	}

	if d.mPlayer.GameMode != packet.GameTypeSurvival && d.mPlayer.GameMode != packet.GameTypeAdventure {
		return
	}

	if d.mPlayer.StackLatency.Milliseconds() > maxPingMs {
		return
	}

	if d.mPlayer.TicksSinceAttack() > 100 {
		return
	}

	currentYaw := d.mPlayer.Movement().Rotation().Z()

	// init player data if not exists
	if !d.initialized {
		d.lastYaw = currentYaw
		d.initialized = true
		return
	}

	prevYaw := d.lastYaw
	d.lastYaw = currentYaw

	// skips if looking straight up/down
	currentPitch := d.mPlayer.Movement().Rotation().X()
	if math32.Abs(currentPitch) >= 89 {
		return
	}

	// skips if player is in liquid
	// Skip if player is in liquid (water or lava)
	blockAtPlayer := d.mPlayer.World().Block(df_cube.Pos(cube.PosFromVec3(d.mPlayer.Movement().Pos())))
	if _, isWater := blockAtPlayer.(block.Water); isWater {
		return
	}
	if _, isLava := blockAtPlayer.(block.Lava); isLava {
		return
	}

	// Skip if player is on stairs
	blockBelowPos := df_cube.Pos(cube.PosFromVec3(d.mPlayer.Movement().Pos().Sub(mgl32.Vec3{0, 1, 0})))
	blockBelow := d.mPlayer.World().Block(blockBelowPos)
	if _, isStairs := blockBelow.(block.Stairs); isStairs {
		return
	}

	yawDelta := math32.Abs(currentYaw - prevYaw)

	// skip for invalid rotations
	if yawDelta < epsilon || yawDelta > 180 {
		return
	}

	d.mPlayer.Dbg.Notify(
		player.DebugModeAimB,
		true,
		"yawDelta=%f samples=%d/%d",
		yawDelta,
		len(d.rotationSamples),
		rotationSamples,
	)

	// check if this rotation delta already exists (pattern detection)
	for _, sample := range d.rotationSamples {
		if math32.Abs(sample-yawDelta) < epsilon {
			return // already seen this rotation
		}
	}

	// add new rotation sample
	d.rotationSamples = append(d.rotationSamples, yawDelta)

	// process check when we have enough samples
	if len(d.rotationSamples) == rotationSamples {
		samples := make([]float32, len(d.rotationSamples))
		copy(samples, d.rotationSamples)
		sort.Slice(samples, func(i, j int) bool {
			return samples[i] < samples[j]
		})

		bestSlope, matchAmount := d.calculateMostFrequentSlope(samples)

		d.mPlayer.Dbg.Notify(
			player.DebugModeAimB,
			true,
			"slope=%f matchAmount=%d",
			bestSlope,
			matchAmount,
		)

		// flag if our slope is too consistent (this is a common aimassist pattern)
		if bestSlope < 0.007 && matchAmount <= 4 {
			d.mPlayer.FailDetection(d, "slope", bestSlope, "matchAmount", matchAmount)
			d.mPlayer.Log().Debug("aim(B)", "slope", bestSlope, "matchAmount", matchAmount, "vl", d.metadata.Violations)
			// Reset samples after flagging
			d.rotationSamples = d.rotationSamples[:0]
			return
		}

		// get rid of oldest sample to maintain buffer size
		d.rotationSamples = d.rotationSamples[1:]
	}
}

func (d *AimB) calculateMostFrequentSlope(sortedRotations []float32) (float32, int) {
	if len(sortedRotations) < 2 {
		return 0.0, 0
	}

	// calc slopes between consecutive rotations
	slopes := make([]float32, 0, len(sortedRotations)-1)
	for i := 0; i < len(sortedRotations)-1; i++ {
		slopes = append(slopes, sortedRotations[i+1]-sortedRotations[i])
	}
	sort.Slice(slopes, func(i, j int) bool {
		return slopes[i] < slopes[j]
	})

	// find the most frequent slope
	bestSlope := float32(0.0)
	currentSlope := float32(math.MaxFloat32)
	bestCount := 0
	currentCount := 0

	for _, slope := range slopes {
		if math32.Abs(slope-currentSlope) < epsilon {
			currentCount++
		} else {
			currentSlope = slope
			currentCount = 1
		}

		if currentCount > bestCount {
			bestCount = currentCount
			bestSlope = currentSlope
		}
	}

	return bestSlope, bestCount
}
