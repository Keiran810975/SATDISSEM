package model

import "sort"

type TimeInterval struct {
	Start float64
	End   float64
}

type SatelliteNode struct {
	ID               int
	MaxConnections   int
	Fragments        map[int]struct{}
	DynamicNeighbors map[int][]TimeInterval
	Bandwidths       map[int]float64
	//Receiving        map[int]bool
}

func NewSatelliteNode(id int) *SatelliteNode {
	return &SatelliteNode{
		ID:               id,
		MaxConnections:   6,
		Fragments:        make(map[int]struct{}),
		DynamicNeighbors: make(map[int][]TimeInterval),
		Bandwidths:       make(map[int]float64),
	}
}

func (s *SatelliteNode) IsConnected(otherID int, currentTime float64) bool {
	intervals := s.DynamicNeighbors[otherID]
	for _, iv := range intervals {
		if iv.Start <= currentTime && currentTime < iv.End {
			return true
		}
	}
	return false
}

func (s *SatelliteNode) GetBandwidth(otherID int) float64 {
	if bw, ok := s.Bandwidths[otherID]; ok {
		return bw
	}
	return 0.0
}

func FragmentList(fragments map[int]struct{}) []int {
	keys := make([]int, 0, len(fragments))
	for k := range fragments {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func FragmentDiff(a, b map[int]struct{}) map[int]struct{} {
	diff := make(map[int]struct{})
	for k := range a {
		if _, exists := b[k]; !exists {
			diff[k] = struct{}{}
		}
	}
	return diff
}
