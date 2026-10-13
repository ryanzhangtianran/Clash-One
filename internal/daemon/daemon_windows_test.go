package daemon

import (
	"strings"
	"testing"
)

func TestScripts(t *testing.T) {
	s := installScript(`C:\Program Files\Connaio\connaio-core.exe`, `C:\Temp\stage`)
	for _, want := range []string{"sc create " + Service, "sc config " + Service, "-ext-ctl-pipe " + Socket, `copy /Y "C:\Program Files\Connaio\connaio-core.exe"`, "sc start " + Service} {
		if !strings.Contains(s, want) {
			t.Errorf("install script lacks %q:\n%s", want, s)
		}
	}
	if u := uninstallScript(); !strings.Contains(u, "sc delete "+Service) {
		t.Errorf("uninstall script:\n%s", u)
	}
}
