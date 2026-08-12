package sdk

import "sort"

type rankedItem struct {
	index int
	key   string
	item  any
}

// RankByLearning reorders items: last success → higher historical bytes/sec →
// fewer consecutive failures → original order (SPEC §12.7).
func RankByLearning[T any](items []T, keyOf func(T) string, state *UpdateState) []T {
	if len(items) <= 1 {
		out := make([]T, len(items))
		copy(out, items)
		return out
	}
	if state == nil {
		state = &UpdateState{}
	}
	indexed := make([]rankedItem, len(items))
	for i, item := range items {
		indexed[i] = rankedItem{index: i, key: keyOf(item), item: item}
	}
	sort.SliceStable(indexed, func(i, j int) bool {
		a, b := indexed[i], indexed[j]
		aLast := state.LastSuccessfulSourceKey == a.key
		bLast := state.LastSuccessfulSourceKey == b.key
		if aLast != bLast {
			return aLast
		}
		aBps, bBps := -1, -1
		if st := state.SourceStats[a.key]; st != nil && st.LastBytesPerSecond != nil {
			aBps = *st.LastBytesPerSecond
		}
		if st := state.SourceStats[b.key]; st != nil && st.LastBytesPerSecond != nil {
			bBps = *st.LastBytesPerSecond
		}
		if aBps != bBps {
			return aBps > bBps
		}
		aFail, bFail := 0, 0
		if st := state.SourceStats[a.key]; st != nil {
			aFail = st.ConsecutiveFailures
		}
		if st := state.SourceStats[b.key]; st != nil {
			bFail = st.ConsecutiveFailures
		}
		if aFail != bFail {
			return aFail < bFail
		}
		return a.index < b.index
	})
	out := make([]T, len(indexed))
	for i, row := range indexed {
		out[i] = row.item.(T)
	}
	return out
}

// RankURLStrings reorders URL strings by learning stats.
func RankURLStrings(urls []string, state *UpdateState) []string {
	return RankByLearning(urls, func(u string) string { return u }, state)
}

// DirectoryServiceKey is the preference key for a directory service id.
func DirectoryServiceKey(id string) string {
	return "service:" + id
}
