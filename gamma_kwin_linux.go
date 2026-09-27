package main

// Gamma control for KDE Plasma on Wayland. KWin has no gamma protocol, but it applies the VCGT
// curve of an output's ICC profile, so we generate a profile holding the wanted gamma curve and
// assign it with kscreen-doctor.
//
// KScreen persists the profile assignment, so the original settings are also written to a state
// file. If the process dies without restoring them, the next run restores them on startup.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type kwinOutput struct {
	Name   string `json:"name"`
	ICC    string `json:"icc"`
	Source string `json:"source"`
}

type kwinGamma struct {
	outputs    []kwinOutput // original settings of the outputs we manage
	profileDir string
	statePath  string
	toggle     int
	applied    bool
}

func newKWinGamma() (*kwinGamma, error) {
	if _, err := exec.LookPath("kscreen-doctor"); err != nil {
		return nil, errors.New("kscreen-doctor not found; it ships with KDE Plasma (libkscreen)")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	k := &kwinGamma{
		profileDir: filepath.Join(cacheDir, "osu-gamma-controller"),
		statePath:  filepath.Join(stateDir, "osu-gamma-controller", "kwin-original-profiles.json"),
	}

	if data, err := os.ReadFile(k.statePath); err == nil {
		if err := json.Unmarshal(data, &k.outputs); err != nil {
			return nil, fmt.Errorf("reading %s: %w", k.statePath, err)
		}
		log.Println("Found color profiles left over from an unclean shutdown, restoring them")
		k.applied = true
		if err := k.restore(); err != nil {
			return nil, err
		}
	}

	outputs, err := queryKScreenOutputs()
	if err != nil {
		return nil, err
	}
	k.outputs = outputs
	if len(k.outputs) == 0 {
		return nil, errors.New("KWin reports no enabled output that supports ICC profiles")
	}
	for _, o := range k.outputs {
		log.Printf("KWin output %s: color profile source %s, ICC profile %q", o.Name, o.Source, o.ICC)
	}
	return k, nil
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// queryKScreenOutputs parses `kscreen-doctor -o` and returns the enabled, connected outputs that
// accept ICC profiles, along with their current profile settings.
func queryKScreenOutputs() ([]kwinOutput, error) {
	out, err := exec.Command("kscreen-doctor", "-o").Output()
	if err != nil {
		return nil, fmt.Errorf("kscreen-doctor -o: %w", err)
	}
	type parsed struct {
		kwinOutput
		enabled, connected, iccCapable bool
	}
	var all []*parsed
	var cur *parsed
	for _, line := range strings.Split(ansiEscape.ReplaceAllString(string(out), ""), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "Output:"); ok {
			fields := strings.Fields(rest) // <id> <name> [<uuid>]
			if len(fields) < 2 {
				cur = nil
				continue
			}
			cur = &parsed{kwinOutput: kwinOutput{Name: fields[1]}}
			all = append(all, cur)
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case line == "enabled":
			cur.enabled = true
		case line == "connected":
			cur.connected = true
		case strings.HasPrefix(line, "ICC profile:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "ICC profile:"))
			cur.iccCapable = v != "incapable"
			if v != "none" && v != "incapable" {
				cur.ICC = v
			}
		case strings.HasPrefix(line, "Color profile source:"):
			cur.Source = strings.TrimSpace(strings.TrimPrefix(line, "Color profile source:"))
		}
	}
	var outputs []kwinOutput
	for _, p := range all {
		if p.enabled && p.connected && p.iccCapable {
			if p.Source == "" {
				p.Source = "sRGB"
			}
			outputs = append(outputs, p.kwinOutput)
		}
	}
	return outputs, nil
}

func (k *kwinGamma) Name() string { return "KWin ICC profile (KDE Plasma Wayland)" }

func (k *kwinGamma) Set(gamma float64) error {
	if err := validateGamma(gamma); err != nil {
		return err
	}
	if gamma == 1 {
		return k.restore()
	}
	if !k.applied {
		if err := k.saveState(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(k.profileDir, 0o755); err != nil {
		return err
	}
	// Alternate between two files so KScreen always sees a new path and reloads the profile.
	k.toggle ^= 1
	path := filepath.Join(k.profileDir, fmt.Sprintf("gamma-%d.icc", k.toggle))
	if err := os.WriteFile(path, buildGammaICC(gamma), 0o644); err != nil {
		return err
	}
	var ops []string
	for _, o := range k.outputs {
		ops = append(ops, "output."+o.Name+".iccprofile."+path, "output."+o.Name+".colorProfileSource.ICC")
	}
	k.applied = true
	return runKScreenDoctor(ops)
}

func (k *kwinGamma) restore() error {
	if !k.applied {
		return nil
	}
	var ops []string
	for _, o := range k.outputs {
		ops = append(ops, "output."+o.Name+".iccprofile."+o.ICC, "output."+o.Name+".colorProfileSource."+o.Source)
	}
	if err := runKScreenDoctor(ops); err != nil {
		return err
	}
	k.applied = false
	if err := os.Remove(k.statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (k *kwinGamma) saveState() error {
	data, err := json.MarshalIndent(k.outputs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(k.statePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(k.statePath, data, 0o644)
}

func runKScreenDoctor(ops []string) error {
	out, err := exec.Command("kscreen-doctor", ops...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kscreen-doctor %s: %w: %s", strings.Join(ops, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k *kwinGamma) Close() error { return k.restore() }
