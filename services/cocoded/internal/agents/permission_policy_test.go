package agents

import "testing"

func TestPermissionRiskForAction(t *testing.T) {
	t.Parallel()

	tests := map[PermissionAction]PermissionRisk{
		PermissionRead:    PermissionRiskLow,
		PermissionSearch:  PermissionRiskLow,
		PermissionTest:    PermissionRiskMedium,
		PermissionShell:   PermissionRiskHigh,
		PermissionWrite:   PermissionRiskHigh,
		PermissionPublish: PermissionRiskCritical,
	}
	for action, want := range tests {
		if got := PermissionRiskForAction(action); got != want {
			t.Fatalf("PermissionRiskForAction(%s) = %s, want %s", action, got, want)
		}
	}
}

func TestReviewModePermissionPolicy(t *testing.T) {
	t.Parallel()

	evaluation := ReviewModePermissionPolicy().Evaluate([]PermissionAction{
		PermissionRead,
		PermissionShell,
		PermissionWrite,
		PermissionPublish,
	})
	if len(evaluation.Results) != 4 {
		t.Fatalf("results = %+v", evaluation.Results)
	}
	if evaluation.Results[0].Decision != PermissionApproved ||
		evaluation.Results[1].Decision != PermissionApproved ||
		evaluation.Results[2].Decision != PermissionDenied ||
		evaluation.Results[3].Decision != PermissionDenied {
		t.Fatalf("evaluation = %+v", evaluation)
	}
	denied, ok := evaluation.FirstDenied()
	if !ok || denied.Action != PermissionWrite {
		t.Fatalf("FirstDenied() = %+v, %v", denied, ok)
	}
}

func TestRequiredPermissionsForRun(t *testing.T) {
	t.Parallel()

	got := RequiredPermissionsForRun(ConnectionConfig{Kind: AdapterCLINonInteractive}, AgentCapabilities{
		CanRead:  true,
		CanWrite: true,
	})
	want := []PermissionAction{PermissionRead, PermissionShell, PermissionWrite}
	if len(got) != len(want) {
		t.Fatalf("RequiredPermissionsForRun() = %+v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("RequiredPermissionsForRun()[%d] = %s, want %s; got %+v", index, got[index], want[index], got)
		}
	}
}
