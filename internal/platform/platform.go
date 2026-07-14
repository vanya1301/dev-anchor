package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type OS string

const (
	MacOS OS = "macos"
	Linux OS = "linux"
)

type Distro string

const (
	UnknownDistro Distro = ""
	Debian        Distro = "debian"
	Ubuntu        Distro = "ubuntu"
	Fedora        Distro = "fedora"
	Arch          Distro = "arch"
)

type PkgManager string

const (
	Brew   PkgManager = "brew"
	Apt    PkgManager = "apt"
	Dnf    PkgManager = "dnf"
	Pacman PkgManager = "pacman"
)

type Info struct {
	OS      OS
	Distro  Distro
	Arch    string
	WSL     bool
	Manager PkgManager
}

func parseOSReleaseID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			v := strings.TrimPrefix(line, "ID=")
			return strings.Trim(v, "\"")
		}
	}
	return ""
}

func DetectFrom(goos, goarch, osRelease string, wslMarker bool) (Info, error) {
	info := Info{Arch: goarch}
	switch goos {
	case "darwin":
		info.OS = MacOS
		info.Manager = Brew
		return info, nil
	case "linux":
		info.OS = Linux
		info.WSL = wslMarker
	default:
		return Info{}, fmt.Errorf("unsupported OS: %s", goos)
	}

	id := parseOSReleaseID(osRelease)
	switch id {
	case "ubuntu":
		info.Distro, info.Manager = Ubuntu, Apt
	case "debian":
		info.Distro, info.Manager = Debian, Apt
	case "fedora":
		info.Distro, info.Manager = Fedora, Dnf
	case "arch":
		info.Distro, info.Manager = Arch, Pacman
	default:
		// ID_LIKE fallback for debian derivatives
		if strings.Contains(osRelease, "ID_LIKE=debian") || strings.Contains(osRelease, "debian") {
			info.Distro, info.Manager = Debian, Apt
			return info, nil
		}
		return Info{}, fmt.Errorf("unsupported linux distro: %q", id)
	}
	return info, nil
}

func Detect() (Info, error) {
	var osRelease string
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		osRelease = string(b)
	}
	wsl := false
	if b, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(strings.ToLower(string(b)), "microsoft") {
			wsl = true
		}
	}
	return DetectFrom(runtime.GOOS, runtime.GOARCH, osRelease, wsl)
}
