package proxy

import (
	"testing"

	"resultproxy-wails/internal/logger"
)

func TestBuildStatusLocked_KillSwitchEmergencyTrueWhenDeadAndArmed(t *testing.T) {
	m := &Manager{
		log:        logger.New(),
		engine:     &stubEngine{},
		connected:  true,
		proxyDead:  true,
		killSwitch: true,
		isAdmin:    true,
	}

	status := m.buildStatusLocked(0, 0, 0, 0, 0)
	if !status.KillSwitchEmergency {
		t.Fatalf("expected killSwitchEmergency=true, got %+v", status)
	}
}

// Without admin the OS firewall can't actually block traffic, so a dead upstream
// must NOT raise the blocking emergency — it only surfaces as IsProxyDead. This
// is the proxy-mode/no-admin false alarm we fixed.
func TestBuildStatusLocked_KillSwitchEmergencyFalseWhenNotAdmin(t *testing.T) {
	m := &Manager{
		log:        logger.New(),
		engine:     &stubEngine{},
		connected:  true,
		proxyDead:  true,
		killSwitch: true,
		isAdmin:    false,
	}

	status := m.buildStatusLocked(0, 0, 0, 0, 0)
	if status.KillSwitchEmergency {
		t.Fatalf("expected killSwitchEmergency=false without admin, got %+v", status)
	}
	if !status.IsProxyDead {
		t.Fatalf("expected isProxyDead=true to still surface, got %+v", status)
	}
}

func TestBuildStatusLocked_KillSwitchEmergencyFalseWhenNotArmed(t *testing.T) {
	m := &Manager{
		log:        logger.New(),
		engine:     &stubEngine{},
		connected:  true,
		proxyDead:  true,
		killSwitch: false,
	}

	status := m.buildStatusLocked(0, 0, 0, 0, 0)
	if status.KillSwitchEmergency {
		t.Fatalf("expected killSwitchEmergency=false, got %+v", status)
	}
}
