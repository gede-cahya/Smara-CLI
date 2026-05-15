package cloud

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestConflictResolverBoundedProperty(t *testing.T) {
	policies := []ConflictPolicy{
		PolicyLastWriteWins,
		PolicyArchiveLoser,
		PolicyManual,
		PolicyMergeContent,
	}

	rapid.Check(t, func(t *rapid.T) {
		policy := rapid.SampledFrom(policies).Draw(t, "policy")
		cloudID := rapid.StringMatching(`[a-z0-9-]{1,40}`).Draw(t, "cloud_id")
		localDevice := rapid.StringMatching(`[a-z0-9-]{1,24}`).Draw(t, "local_device")
		remoteDevice := rapid.StringMatching(`[a-z0-9-]{1,24}`).Draw(t, "remote_device")
		if remoteDevice == localDevice {
			remoteDevice += "-r"
		}

		base := rapid.Int64Range(0, 4_000_000_000).Draw(t, "base_unix")
		localOffset := rapid.Int64Range(-86_400, 86_400).Draw(t, "local_offset")
		remoteOffset := rapid.Int64Range(-86_400, 86_400).Draw(t, "remote_offset")

		local := MemoryRow{
			ID:        rapid.Int64Range(1, 1_000_000).Draw(t, "local_id"),
			CloudID:   cloudID,
			Content:   rapid.StringMatching(`[A-Za-z0-9 _.,:;!?@#%&()\[\]\-]{0,120}`).Draw(t, "local_content"),
			DeviceID:  localDevice,
			Version:   rapid.IntRange(0, 10_000).Draw(t, "local_version"),
			UpdatedAt: time.Unix(base+localOffset, rapid.Int64Range(0, 999_999_999).Draw(t, "local_nanos")).UTC(),
		}
		remote := MemoryRow{
			ID:        rapid.Int64Range(1, 1_000_000).Draw(t, "remote_id"),
			CloudID:   cloudID,
			Content:   rapid.StringMatching(`[A-Za-z0-9 _.,:;!?@#%&()\[\]\-]{0,120}`).Draw(t, "remote_content"),
			DeviceID:  remoteDevice,
			Version:   rapid.IntRange(0, 10_000).Draw(t, "remote_version"),
			UpdatedAt: time.Unix(base+remoteOffset, rapid.Int64Range(0, 999_999_999).Draw(t, "remote_nanos")).UTC(),
		}

		resolver, err := NewResolver(policy)
		if err != nil {
			t.Fatalf("NewResolver(%q): %v", policy, err)
		}

		winner1, loser1, err1 := resolver.Resolve(local, remote)
		winner2, loser2, err2 := resolver.Resolve(local, remote)

		if policy == PolicyManual {
			if !errors.Is(err1, ErrManualConflict) {
				t.Fatalf("manual policy must return ErrManualConflict, got %v", err1)
			}
			if !reflect.DeepEqual(winner1, local) || loser1 != nil {
				t.Fatalf("manual policy changed row or emitted loser: winner=%+v loser=%+v", winner1, loser1)
			}
			if !errors.Is(err2, ErrManualConflict) || !reflect.DeepEqual(winner1, winner2) || !reflect.DeepEqual(loser1, loser2) {
				t.Fatalf("manual policy is not deterministic: (%+v,%+v,%v) vs (%+v,%+v,%v)", winner1, loser1, err1, winner2, loser2, err2)
			}
			return
		}

		if err1 != nil || err2 != nil {
			t.Fatalf("policy %q unexpected errors: %v / %v", policy, err1, err2)
		}
		if !reflect.DeepEqual(winner1, winner2) || !reflect.DeepEqual(loser1, loser2) {
			t.Fatalf("policy %q is not deterministic: (%+v,%+v) vs (%+v,%+v)", policy, winner1, loser1, winner2, loser2)
		}
		if winner1.ID != local.ID || winner1.CloudID != cloudID {
			t.Fatalf("policy %q winner identity mismatch: %+v", policy, winner1)
		}

		switch policy {
		case PolicyLastWriteWins, PolicyArchiveLoser:
			if loser1 == nil {
				t.Fatalf("policy %q must preserve loser", policy)
			}
			expectedWinner, expectedLoser := pickLWWWinner(local, remote)
			if winner1.Content != expectedWinner.Content || winner1.DeviceID != expectedWinner.DeviceID || winner1.Version != expectedWinner.Version || !winner1.UpdatedAt.Equal(expectedWinner.UpdatedAt) {
				t.Fatalf("policy %q winner mismatch: got %+v expected source %+v", policy, winner1, expectedWinner)
			}
			if loser1.MemoryID != local.ID || loser1.Content != expectedLoser.Content || loser1.Version != expectedLoser.Version || loser1.ChangedBy != expectedLoser.DeviceID || loser1.Reason != "conflict-loser" {
				t.Fatalf("policy %q loser mismatch: got %+v expected source %+v", policy, loser1, expectedLoser)
			}
		case PolicyMergeContent:
			if loser1 != nil {
				t.Fatalf("merge-content preserves both contents inline and should not archive loser, got %+v", loser1)
			}
			if !strings.Contains(winner1.Content, local.Content) || !strings.Contains(winner1.Content, remote.Content) {
				t.Fatalf("merge-content winner does not contain both inputs: %q", winner1.Content)
			}
			if strings.Count(winner1.Content, "---merged from device") != 1 {
				t.Fatalf("merge-content separator count mismatch: %q", winner1.Content)
			}
			expectedVersion := local.Version
			if remote.Version > expectedVersion {
				expectedVersion = remote.Version
			}
			if winner1.Version != expectedVersion+1 {
				t.Fatalf("merge-content version mismatch: got %d expected %d", winner1.Version, expectedVersion+1)
			}
		}
	})
}
