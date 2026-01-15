package main

import (
	"fmt"
	"sate/algorithms"

	"time"

	"sate/globals"
	"sate/model"
)

var (
	NUM_SATELLITES   = globals.N - 1 
	NUM_FRAGMENTS    = globals.F() 
	FRAGMENT_SIZE_MB = globals.FRAGMENT_SIZE_MB 
)

func PrintDynamicTopology(nodes []*model.SatelliteNode) {
	fmt.Println("📡 Dynamic connection topology time windows:")
	for _, node := range nodes {
		for neighborID, intervals := range node.DynamicNeighbors {
			// To avoid duplicate printing of bidirectional relationships, only print the pair with smaller ID
			if node.ID < neighborID {
				fmt.Printf("Node %d <-> Node %d:\n", node.ID, neighborID)
				for _, iv := range intervals {
					fmt.Printf("  [%.2f, %.2f) seconds\n", iv.Start, iv.End)
				}
			}
		}
	}
}

func main() {
	totalNodes := NUM_SATELLITES + 1
	topologyFile := "intervals384.json"
	// topologyFile := "intervals576.json"
	// topologyFile := "intervals768.json"
	// topologyFile := "intervals1024.json"
	// topologyFile := "intervals1536.json"

	nodes := globals.LoadDynamicTopologyFromFile(topologyFile, totalNodes)
	for i := 0; i < NUM_FRAGMENTS; i++ {
		nodes[0].Fragments[i] = struct{}{}
	}

	fmt.Printf("Simulation started: %d satellites, %.0f MB per fragment, %d fragments total.\n", NUM_SATELLITES, FRAGMENT_SIZE_MB, NUM_FRAGMENTS)
	fmt.Println("Simulating data transmission...")

	startTime := time.Now()

	// algorithms.RunSimulationFlooding(nodes)  // Flooding
	algorithms.RunSimulationExperiment(nodes) // Experiment
	// algorithms.RunSimulationRLNC(nodes) // RLNC
	// algorithms.RunSimulationDFedSat(nodes) // DFedSat

	// algorithms.RunAblationStudy(nodes)

	endTime := time.Now()
	duration := endTime.Sub(startTime).Seconds()

	fmt.Printf("\nTotal Time：%.2f s\n", duration)
}
