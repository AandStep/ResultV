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
	}

	status := m.buildStatusLocked(0, 0, 0, 0, 0)
	if !status.KillSwitchEmergency {
		t.Fatalf("expected killSwitchEmergency=true, got %+v", status)
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
