package target

import "fmt"

type OS string

type Arch string

const (
	OSLinux  OS = "linux"
	OSDarwin OS = "darwin"
)

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

type Target struct {
	OS   OS
	Arch Arch
}

func Parse(osName, archName string) (Target, error) {
	t := Target{OS: OS(osName), Arch: Arch(archName)}
	if err := t.Validate(); err != nil {
		return Target{}, err
	}
	return t, nil
}

func (t Target) Validate() error {
	switch {
	case t.OS == OSLinux && t.Arch == ArchAMD64:
		return nil
	case t.OS == OSDarwin && (t.Arch == ArchAMD64 || t.Arch == ArchARM64):
		return nil
	default:
		return fmt.Errorf("unsupported target: %s/%s", t.OS, t.Arch)
	}
}

func (t Target) String() string {
	return string(t.OS) + "/" + string(t.Arch)
}
