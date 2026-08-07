package overlay

import (
	"io"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"

	"github.com/nospy/albion-openradar/internal/logger"
)

// audioSampleRate matches the coin.mp3/player.mp3 assets closely enough for mp3.Decode's
// internal resampling to sound correct - a fixed, conventional rate (44.1kHz), not derived from
// the files themselves since ebiten's audio.Context is a process-wide singleton created once
// before any file is read.
const audioSampleRate = 44100

// alertPlayer plays the same two alert sounds the web app does (HarvestablesHandler.js's
// coin.mp3 for a matched resource, PlayersHandler.js's player.mp3 for a hostile player) -
// reusing the exact same asset files from web/sounds/, read directly off disk via appDir like
// internal/gamedata's loaders do, rather than plumbing the embedded asset FS through (this
// package never needed an assets dependency before). One ebiten/audio.Context per process (it
// panics if constructed twice), decoded once into raw PCM at startup; each play() call spins up
// a fresh short-lived audio.Player from that PCM so overlapping/rapid alerts don't fight over a
// single player's playhead.
type alertPlayer struct {
	ctx       *audio.Context
	coinPCM   []byte
	playerPCM []byte
}

func newAlertPlayer(appDir string) *alertPlayer {
	ctx := audio.NewContext(audioSampleRate)
	return &alertPlayer{
		ctx:       ctx,
		coinPCM:   loadPCM(ctx, filepath.Join(appDir, "web", "sounds", "coin.mp3")),
		playerPCM: loadPCM(ctx, filepath.Join(appDir, "web", "sounds", "player.mp3")),
	}
}

func loadPCM(ctx *audio.Context, path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		logger.PrintWarn("OVERLAY", "alert sound %s not found, alerts will be silent: %v", path, err)
		return nil
	}
	defer f.Close()

	stream, err := mp3.Decode(ctx, f)
	if err != nil {
		logger.PrintWarn("OVERLAY", "decode alert sound %s failed: %v", path, err)
		return nil
	}
	data, err := io.ReadAll(stream)
	if err != nil {
		logger.PrintWarn("OVERLAY", "read alert sound %s failed: %v", path, err)
		return nil
	}
	return data
}

func (a *alertPlayer) playResourceFound() { a.play(a.coinPCM) }
func (a *alertPlayer) playHostilePlayer() { a.play(a.playerPCM) }
func (a *alertPlayer) play(pcm []byte) {
	if pcm == nil {
		return
	}
	a.ctx.NewPlayerFromBytes(pcm).Play()
}
