package sixdegrees

// Wrapper to call the BFS implementation in the root package so that
// sixDegrees package tests and web code can reference RunSearchOpts here.
// The root package defines RunSearchOpts with the actual logic.
// We import via a blank import to avoid cycles; since root package is main,
// we instead declare a thin implementation here that mirrors the logic needed
// for tests using only in-memory Artists data (no API calls).

// RunSearchOpts performs a bounded/unbounded BFS search between artists using
// only the already-populated Tracks on the provided Artists. This avoids any
// external API calls and is suitable for unit tests.
func RunSearchOpts(start, target *Artists, maxDepth int, verbose bool, _ ...interface{}) (*Helper, []string, []string, bool) {
	h := NewHelper()
	h.ArtistMap[start.Name] = start
	h.DistTo[start.Name] = 0

	queue := []*Artists{start}
	visited := map[string]bool{start.Name: true}
	found := false

	for len(queue) > 0 && !found {
		current := queue[0]
		queue = queue[1:]

		if maxDepth >= 0 && h.DistTo[current.Name] >= maxDepth {
			continue
		}

		for _, tr := range current.Tracks {
			if tr.Artist.Name == target.Name {
				h.Prev[target.Name] = current.Name
				h.Evidence[target.Name] = tr.Name
				found = true
				break
			}
			for _, feat := range tr.Featured {
				if feat == nil || feat.Name == "" || feat.Name == current.Name {
					continue
				}
				if visited[feat.Name] {
					continue
				}
				visited[feat.Name] = true
				h.Prev[feat.Name] = current.Name
				h.Evidence[feat.Name] = tr.Name
				h.DistTo[feat.Name] = h.DistTo[current.Name] + 1
				h.ArtistMap[feat.Name] = feat
				if feat.Name == target.Name {
					found = true
					break
				}
				queue = append(queue, feat)
			}
			if found {
				break
			}
		}
	}

	// Always return helper; only build path if target found
	if found {
		path, songs := h.ReconstructPath(start.Name, target.Name)
		return h, path, songs, true
	}
	return h, nil, nil, false
}
