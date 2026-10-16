package desktop

import (
	"math"
	"time"

	"github.com/knothhe/passpine/client/app"
)

type generatorState struct {
	length                                  float64
	lower, upper, numbers, symbols, exclude bool
	password                                string
	open                                    bool
	target                                  *editorIdentity
	request                                 uint64
	busy, pending                           bool
	inflight                                uint64
	generated                               generatorOptions
	done                                    func(string)
	generation                              uint64
	vaultID                                 string
}

type generatorOptions struct {
	length                                  int
	lower, upper, numbers, symbols, exclude bool
}

func (g *generatorState) options() generatorOptions {
	length := math.Round(g.length)
	if math.IsNaN(length) || math.IsInf(length, 0) {
		length = 24
	}
	g.length = min(128, max(8, length))
	return generatorOptions{int(g.length), g.lower, g.upper, g.numbers, g.symbols, g.exclude}
}

func (o generatorOptions) valid() bool {
	return o.lower || o.upper || o.numbers || o.symbols
}

func (a *application) generatorCurrent() bool {
	g := &a.generator
	return g.open && !a.closing && !a.state.Locked && a.editor.open && g.target != nil &&
		g.target == a.editor.identity && g.generation == a.generation && g.vaultID == a.state.VaultID
}

func (a *application) generatorReady() bool {
	g := &a.generator
	options := g.options()
	return a.generatorCurrent() && !g.busy && !g.pending && g.password != "" && options.valid() && g.generated == options
}

// Generation has its own request lane. Editing options never disables the
// application's controls or replaces an unrelated metadata response.
func (a *application) generate(done func(string)) {
	if !a.generatorCurrent() {
		return
	}
	g := &a.generator
	g.options()
	g.request++
	g.pending = true
	g.done = done
	a.lastInput = time.Now()
	a.flushGeneratorUpdate()
	a.invalidate()
}

func (a *application) requestGeneratorUpdate() {
	a.generate(nil)
}

func (a *application) flushGeneratorUpdate() {
	g := &a.generator
	if !g.pending || g.busy || !a.generatorCurrent() {
		return
	}
	options := g.options()
	g.pending = false
	if !options.valid() {
		g.password = ""
		g.generated = generatorOptions{}
		g.done = nil
		return
	}
	request, target, generation, vaultID, done := g.request, g.target, g.generation, g.vaultID, g.done
	g.busy, g.inflight = true, request
	req := app.Request{
		Action: "generate", VaultID: vaultID, Length: options.length,
		Lowercase: options.lower, Uppercase: options.upper, Numbers: options.numbers,
		Symbols: options.symbols, ExcludeAmbiguous: options.exclude,
	}
	go func() {
		response := a.service.Execute(req)
		a.runOnMain(func() {
			g := &a.generator
			// Closing and reopening creates a new request lane. An old callback
			// must not clear that lane's busy state or dispatch its pending work.
			if !g.busy || g.inflight != request || g.target != target || g.generation != generation || g.vaultID != vaultID {
				return
			}
			g.busy, g.inflight = false, 0
			if !a.generatorCurrent() {
				a.closeGenerator()
				a.invalidate()
				return
			}
			if response.Code == "vault_changed" || response.Data.VaultID != "" && response.Data.VaultID != vaultID {
				a.invalidateGeneratorSession()
				a.queueMetadataRefresh(true)
				a.invalidate()
				return
			}
			if response.Data.Locked || response.Code == "locked" {
				a.invalidateGeneratorSession()
				a.invalidate()
				return
			}
			if request != g.request || options != g.options() {
				// Multiple slider/number/toggle events collapse to this one latest
				// snapshot; the completed result is never shown or applied.
				if !g.pending {
					g.request++
					g.pending = true
					g.done = nil
				}
				a.flushGeneratorUpdate()
				a.invalidate()
				return
			}
			if response.Status != "ok" || response.Data.Password == "" {
				g.password = ""
				g.generated = generatorOptions{}
				g.done = nil
				a.errorText = response.Message
				if a.errorText == "" {
					a.errorText = response.Code
				}
			} else {
				g.password = response.Data.Password
				g.generated = options
				g.done = nil
				if done != nil {
					done(g.password)
				}
			}
			a.invalidate()
		})
	}()
}

// A generator response can be the first observer of a lock or external vault
// switch. Revoke main-lane snapshots as well as the generated result.
func (a *application) invalidateGeneratorSession() {
	a.generation++
	a.busy, a.backgroundBusy = false, false
	a.clearSecrets()
	a.state.Locked = true
}

func (a *application) openGenerator() {
	a.closeGenerator()
	if !a.editor.open || a.state.Locked || a.closing {
		return
	}
	g := &a.generator
	g.open = true
	g.target = a.editor.ensureIdentity()
	g.generation, g.vaultID = a.generation, a.state.VaultID
	a.generate(nil)
}

func (a *application) closeGenerator() {
	g := &a.generator
	g.open = false
	g.target = nil
	g.password = ""
	g.generated = generatorOptions{}
	g.busy, g.pending, g.inflight = false, false, 0
	g.done = nil
	g.request++
}

func (a *application) applyGeneratedPassword() {
	if !a.generatorReady() {
		return
	}
	a.editor.password = a.generator.password
	a.editor.compose()
	a.lastInput = time.Now()
	a.closeGenerator()
}
