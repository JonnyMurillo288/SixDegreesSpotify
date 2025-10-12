package sixdegrees

import "testing"

// BFS unit tests using a small synthetic graph without hitting Spotify API.
func TestBFS_PathReconstruction(t *testing.T) {
	// Build a small synthetic set of artists and tracks to simulate edges:
	A := &Artists{Name: "A"}
	B := &Artists{Name: "B"}
	C := &Artists{Name: "C"}
	D := &Artists{Name: "D"}

	// Tracks "owned" by an artist with featured collaborators create edges A->B, B->C, C->D
	A.Tracks = []Track{{Artist: A, Name: "t1", Featured: []*Artists{B}}}
	B.Tracks = []Track{{Artist: B, Name: "t2", Featured: []*Artists{C}}}
	C.Tracks = []Track{{Artist: C, Name: "t3", Featured: []*Artists{D}}}

	h := NewHelper()
	h.ArtistMap[A.Name] = A
	h.DistTo[A.Name] = 0

	// Use bfsWithOptions directly with no depth limit
	_, found := h.bfsWithOptions(D.Name, []*Artists{A}, -1, false)
	if !found {
		t.Fatalf("expected to find path from A to D")
	}
	path := h.ReconstructPath(A.Name, D.Name)
	want := []string{"A", "B", "C", "D"}
	if len(path) != len(want) {
		t.Fatalf("expected path len %d, got %d: %v", len(want), len(path), path)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("path mismatch at %d: want %s got %s", i, want[i], path[i])
		}
	}
}

func TestBFS_DepthLimitStopsExpansion(t *testing.T) {
	A := &Artists{Name: "A"}
	B := &Artists{Name: "B"}
	C := &Artists{Name: "C"}

	A.Tracks = []Track{{Artist: A, Name: "t1", Featured: []*Artists{B}}}
	B.Tracks = []Track{{Artist: B, Name: "t2", Featured: []*Artists{C}}}

	h := NewHelper()
	h.ArtistMap[A.Name] = A
	h.DistTo[A.Name] = 0

	// Depth limit of 1 allows A->B but not B->C expansion
	_, found := h.bfsWithOptions(C.Name, []*Artists{A}, 1, false)
	if found {
		t.Fatalf("did not expect to find C within depth 1")
	}
}
