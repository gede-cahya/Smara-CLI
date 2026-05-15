package cloud

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConflictResolverPolicies(t *testing.T) {
	t0 := time.Unix(100, 0).UTC()
	local := MemoryRow{ID: 7, CloudID: "cloud-1", Content: "local-content", DeviceID: "dev-b", Version: 2, UpdatedAt: t0}
	remoteNewer := MemoryRow{ID: 99, CloudID: "cloud-1", Content: "remote-content", DeviceID: "dev-a", Version: 3, UpdatedAt: t0.Add(time.Second)}

	for _, policy := range []ConflictPolicy{PolicyLastWriteWins, PolicyArchiveLoser} {
		r, err := NewResolver(policy)
		if err != nil {
			t.Fatalf("NewResolver(%s): %v", policy, err)
		}
		w1, l1, err := r.Resolve(local, remoteNewer)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", policy, err)
		}
		w2, l2, err := r.Resolve(local, remoteNewer)
		if err != nil {
			t.Fatalf("Resolve second(%s): %v", policy, err)
		}
		if !reflect.DeepEqual(w1, w2) || !reflect.DeepEqual(l1, l2) {
			t.Fatalf("%s not deterministic", policy)
		}
		if w1.ID != local.ID || w1.CloudID != local.CloudID || w1.Content != remoteNewer.Content {
			t.Fatalf("%s winner mismatch: %+v", policy, w1)
		}
		if l1 == nil || l1.MemoryID != local.ID || l1.Content != local.Content || l1.Reason != "conflict-loser" || l1.ChangedBy != local.DeviceID {
			t.Fatalf("%s loser mismatch: %+v", policy, l1)
		}
	}

	// Equal timestamps use deterministic DeviceID lexicographic ascending tiebreaker.
	r, _ := NewResolver(PolicyLastWriteWins)
	winner, loser, err := r.Resolve(local, MemoryRow{ID: 99, CloudID: "cloud-1", Content: "remote-tie", DeviceID: "dev-a", Version: 4, UpdatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	if winner.Content != "remote-tie" || loser == nil || loser.Content != local.Content {
		t.Fatalf("tie-break mismatch winner=%+v loser=%+v", winner, loser)
	}

	manual, _ := NewResolver(PolicyManual)
	winner, loser, err = manual.Resolve(local, remoteNewer)
	if !errors.Is(err, ErrManualConflict) || !reflect.DeepEqual(winner, local) || loser != nil {
		t.Fatalf("manual mismatch winner=%+v loser=%+v err=%v", winner, loser, err)
	}

	merge, _ := NewResolver(PolicyMergeContent)
	winner, loser, err = merge.Resolve(local, remoteNewer)
	if err != nil || loser != nil {
		t.Fatalf("merge err/loser mismatch: loser=%+v err=%v", loser, err)
	}
	if winner.CloudID != local.CloudID || winner.ID != local.ID || winner.Version != remoteNewer.Version+1 {
		t.Fatalf("merge winner fields mismatch: %+v", winner)
	}
	if !strings.Contains(winner.Content, local.Content) || !strings.Contains(winner.Content, remoteNewer.Content) || strings.Count(winner.Content, "---merged from device") != 1 {
		t.Fatalf("merge content mismatch: %q", winner.Content)
	}
}
