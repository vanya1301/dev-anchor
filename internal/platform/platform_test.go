package platform

import "testing"

func TestDetectFrom(t *testing.T) {
	cases := []struct {
		name      string
		goos      string
		goarch    string
		osRelease string
		wsl       bool
		want      Info
	}{
		{
			name:   "macos arm64",
			goos:   "darwin",
			goarch: "arm64",
			want:   Info{OS: MacOS, Distro: UnknownDistro, Arch: "arm64", WSL: false, Manager: Brew},
		},
		{
			name:      "ubuntu",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=ubuntu\nID_LIKE=debian\n",
			want:      Info{OS: Linux, Distro: Ubuntu, Arch: "amd64", WSL: false, Manager: Apt},
		},
		{
			name:      "debian",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=debian\n",
			want:      Info{OS: Linux, Distro: Debian, Arch: "amd64", WSL: false, Manager: Apt},
		},
		{
			name:      "fedora",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=fedora\n",
			want:      Info{OS: Linux, Distro: Fedora, Arch: "amd64", WSL: false, Manager: Dnf},
		},
		{
			name:      "arch",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=arch\n",
			want:      Info{OS: Linux, Distro: Arch, Arch: "amd64", WSL: false, Manager: Pacman},
		},
		{
			name:      "ubuntu under wsl",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=ubuntu\n",
			wsl:       true,
			want:      Info{OS: Linux, Distro: Ubuntu, Arch: "amd64", WSL: true, Manager: Apt},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DetectFrom(c.goos, c.goarch, c.osRelease, c.wsl)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestDetectFromUnknownErrors(t *testing.T) {
	if _, err := DetectFrom("plan9", "amd64", "", false); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
	if _, err := DetectFrom("linux", "amd64", "ID=gentoo\n", false); err == nil {
		t.Fatal("expected error for unsupported distro")
	}
}
