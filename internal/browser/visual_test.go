package browser

import "testing"

func TestPlanResponsiveNavbarVisualCheck(t *testing.T) {
	task, err := Plan("Buka http://localhost:5173 di browser. Periksa navbar responsif mobile tablet desktop dan ambil screenshot")
	if err != nil {
		t.Fatal(err)
	}
	last := task.Steps[len(task.Steps)-1]
	if last.Action != "visual-check" || last.Target != "navbar" {
		t.Fatalf("expected visual-check navbar, got %+v", last)
	}
}

func TestResponsiveViewports(t *testing.T) {
	if len(responsiveViewports) != 3 {
		t.Fatalf("viewports=%d", len(responsiveViewports))
	}
	want := map[string][2]int{"mobile": {375, 812}, "tablet": {768, 1024}, "desktop": {1440, 900}}
	for _, vp := range responsiveViewports {
		size, ok := want[vp.Name]
		if !ok || vp.Width != size[0] || vp.Height != size[1] {
			t.Fatalf("unexpected viewport %+v", vp)
		}
	}
}
