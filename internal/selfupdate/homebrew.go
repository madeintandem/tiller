package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tillerBrewPackage = "madeintandem/tap/tiller"

// HomebrewInstall identifies an installed package verified with its own brew,
// never a brew found on PATH. Keep the value returned by DetectHomebrew intact.
type HomebrewInstall struct {
	Brew    string `json:"brew"`
	Prefix  string `json:"prefix"`
	Package string `json:"package"`
	Kind    string `json:"kind"`

	path       string
	brewPath   string
	brewDigest string
	brewInfo   os.FileInfo
}

type brewMetadata struct {
	Casks []struct {
		Token     string  `json:"token"`
		FullToken string  `json:"full_token"`
		Tap       string  `json:"tap"`
		Installed *string `json:"installed"`
		Artifacts []struct {
			Binary []json.RawMessage `json:"binary"`
			Target string            `json:"target"`
		} `json:"artifacts"`
	} `json:"casks"`
	Formulae []struct {
		Name      string `json:"name"`
		FullName  string `json:"full_name"`
		Tap       string `json:"tap"`
		Installed []struct {
			Version string `json:"version"`
		} `json:"installed"`
	} `json:"formulae"`
}

// DetectHomebrew returns nil only for a non-Homebrew path. A Cellar/Caskroom
// path with missing, foreign or inconsistent owner metadata is an error, never
// permission to overwrite package-manager files directly.
func DetectHomebrew(ctx context.Context, path string) (*HomebrewInstall, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolving installation owner for %s: %w", path, err)
	}
	prefix, kind, version, managed := brewLocation(resolved)
	if !managed {
		if _, _, _, looksManaged := brewLocation(absolute); looksManaged {
			return nil, fmt.Errorf("Homebrew-looking executable %s resolves outside its installation", path)
		}
		return nil, nil
	}
	if prefix == "" || version == "" {
		return nil, fmt.Errorf("cannot verify Homebrew-looking executable %s", resolved)
	}
	install := &HomebrewInstall{Brew: filepath.Join(prefix, "bin", "brew"), Prefix: prefix, Package: tillerBrewPackage, Kind: kind, path: resolved}
	install.brewPath, err = filepath.EvalSymlinks(install.Brew)
	if err != nil {
		return nil, fmt.Errorf("cannot verify owning Homebrew %s: %w", install.Brew, err)
	}
	if !pathWithin(prefix, install.brewPath) {
		return nil, fmt.Errorf("owning Homebrew %s resolves outside prefix %s", install.Brew, prefix)
	}
	install.brewDigest, install.brewInfo, err = executableSnapshot(install.brewPath)
	if err != nil {
		return nil, err
	}
	out, err := install.query(ctx, "--prefix")
	if err != nil {
		return nil, err
	}
	if err := matchingBrewDirectory(out, prefix); err != nil {
		return nil, err
	}
	room := "Cellar"
	if kind == "cask" {
		room = "Caskroom"
	}
	out, err = install.query(ctx, "--"+strings.ToLower(room))
	if err != nil {
		return nil, err
	}
	if err := matchingBrewDirectory(out, filepath.Join(prefix, room)); err != nil {
		return nil, err
	}
	out, err = install.query(ctx, "info", "--json=v2", "--"+kind, tillerBrewPackage)
	if err != nil {
		return nil, err
	}
	var metadata brewMetadata
	if err := json.Unmarshal(out, &metadata); err != nil {
		return nil, fmt.Errorf("reading installed Homebrew metadata: %w", err)
	}
	if !metadata.owns(kind, version) {
		return nil, fmt.Errorf("Homebrew metadata does not identify installed %s %s at %s", kind, tillerBrewPackage, resolved)
	}
	listed := metadata.caskOwnsPath(prefix, version, resolved)
	if kind == "formula" {
		out, err = install.query(ctx, "list", "--formula", tillerBrewPackage)
		if err != nil {
			return nil, err
		}
		for _, candidate := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			// Formula listing is one installed file per line. Cask listing is
			// human prose, so casks use their JSON binary artifacts instead.
			if !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate {
				continue
			}
			actual, err := filepath.EvalSymlinks(candidate)
			if err == nil && actual == resolved {
				listed = true
				break
			}
		}
	}
	if !listed {
		return nil, fmt.Errorf("executable %s is not an installed Homebrew package artifact of %s", resolved, tillerBrewPackage)
	}
	if _, _, err := executableSnapshot(resolved); err != nil {
		return nil, err
	}
	if err := install.checkBrew(); err != nil {
		return nil, err
	}
	return install, nil
}

// UpgradeHomebrew delegates only the verified package. Failure can leave package
// manager changes in place: callers must report that boundary, never swap a file
// into the Cellar/Caskroom as a fallback. The returned path is freshly resolved
// from the owning prefix and verified after brew, including a moved cask path.
func UpgradeHomebrew(ctx context.Context, install *HomebrewInstall) (string, error) {
	if install == nil || install.brewInfo == nil || install.Package != tillerBrewPackage || (install.Kind != "cask" && install.Kind != "formula") {
		return "", fmt.Errorf("Homebrew installation was not verified by DetectHomebrew")
	}
	if err := install.checkBrew(); err != nil {
		return "", err
	}
	current, err := DetectHomebrew(ctx, install.path)
	if err != nil {
		return "", err
	}
	if current == nil || current.Prefix != install.Prefix || current.Kind != install.Kind || current.Package != install.Package {
		return "", fmt.Errorf("Homebrew installation owner changed before delegation")
	}
	if err := install.checkBrew(); err != nil {
		return "", err
	}
	_, err = executableCommand(ctx, 10*time.Minute, install.Brew, brewEnvironment(false), "upgrade", "--"+install.Kind, install.Package)
	if err != nil {
		return "", fmt.Errorf("Homebrew upgrade failed; package-manager state may have changed: %w", err)
	}
	linked := filepath.Join(install.Prefix, "bin", "tiller")
	updated, err := DetectHomebrew(ctx, linked)
	if err != nil {
		return "", fmt.Errorf("Homebrew returned successfully but its resulting executable could not be verified: %w", err)
	}
	if updated == nil || updated.Prefix != install.Prefix || updated.Kind != install.Kind || updated.Package != install.Package {
		return "", fmt.Errorf("Homebrew returned successfully but the resulting executable has a different installation owner")
	}
	identity, err := ReadExecutable(ctx, updated.path)
	if err != nil {
		return "", fmt.Errorf("verifying executable after Homebrew upgrade: %w", err)
	}
	return identity.Path, nil
}

func (i *HomebrewInstall) query(ctx context.Context, args ...string) ([]byte, error) {
	if err := i.checkBrew(); err != nil {
		return nil, err
	}
	return executableCommand(ctx, 30*time.Second, i.Brew, brewEnvironment(true), args...)
}

func (i *HomebrewInstall) checkBrew() error {
	if i.Brew != filepath.Join(i.Prefix, "bin", "brew") {
		return fmt.Errorf("Homebrew executable no longer matches owning prefix")
	}
	resolved, err := filepath.EvalSymlinks(i.Brew)
	if err != nil || resolved != i.brewPath {
		return fmt.Errorf("owning Homebrew executable changed since verification")
	}
	digest, info, err := executableSnapshot(resolved)
	if err != nil {
		return err
	}
	if i.brewInfo == nil || digest != i.brewDigest || !sameExecutableFile(i.brewInfo, info) {
		return fmt.Errorf("owning Homebrew executable changed since verification")
	}
	return nil
}

func (m brewMetadata) owns(kind, version string) bool {
	if kind == "cask" {
		return len(m.Casks) == 1 && m.Casks[0].Token == "tiller" && m.Casks[0].FullToken == tillerBrewPackage && m.Casks[0].Tap == "madeintandem/tap" && m.Casks[0].Installed != nil && *m.Casks[0].Installed == version
	}
	if len(m.Formulae) != 1 || m.Formulae[0].Name != "tiller" || m.Formulae[0].FullName != tillerBrewPackage || m.Formulae[0].Tap != "madeintandem/tap" {
		return false
	}
	for _, installed := range m.Formulae[0].Installed {
		if installed.Version == version {
			return true
		}
	}
	return false
}

func (m brewMetadata) caskOwnsPath(prefix, version, path string) bool {
	if len(m.Casks) != 1 {
		return false
	}
	link := filepath.Join(prefix, "bin", "tiller")
	linked, err := filepath.EvalSymlinks(link)
	if err != nil || linked != path {
		return false
	}
	for _, artifact := range m.Casks[0].Artifacts {
		if len(artifact.Binary) == 0 || artifact.Target != "" && artifact.Target != link {
			continue
		}
		var source string
		if json.Unmarshal(artifact.Binary[0], &source) != nil || source == "." || !filepath.IsLocal(source) || filepath.Clean(source) != source {
			continue
		}
		if filepath.Join(prefix, "Caskroom", "tiller", version, source) == path {
			return true
		}
	}
	return false
}

func brewLocation(path string) (prefix, kind, version string, managed bool) {
	for _, room := range []string{"Caskroom", "Cellar"} {
		before, after, found := strings.Cut(path, string(filepath.Separator)+room+string(filepath.Separator))
		if !found {
			continue
		}
		parts := strings.Split(after, string(filepath.Separator))
		if len(parts) < 3 || parts[0] != "tiller" || parts[1] == "" {
			return "", "", "", true
		}
		kind = "formula"
		if room == "Caskroom" {
			kind = "cask"
		}
		return before, kind, parts[1], true
	}
	return "", "", "", false
}

func matchingBrewDirectory(output []byte, expected string) error {
	path := strings.TrimSpace(string(output))
	actual, err := filepath.EvalSymlinks(path)
	if err != nil || actual != expected {
		return fmt.Errorf("Homebrew metadata directory %q does not match owning path %s", path, expected)
	}
	return nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func brewEnvironment(inspect bool) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if key == "HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK" || key == "HOMEBREW_NO_INSTALL_CLEANUP" || inspect && key == "HOMEBREW_NO_AUTO_UPDATE" {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment, "HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1", "HOMEBREW_NO_INSTALL_CLEANUP=1")
	if inspect {
		environment = append(environment, "HOMEBREW_NO_AUTO_UPDATE=1")
	}
	return environment
}
