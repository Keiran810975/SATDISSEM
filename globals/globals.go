package globals

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sate/model"
	"strconv"
	"strings"
)

var (
	N = 384 + 1
	f = 500

	FRAGMENT_SIZE_MB = 1.0

	NUM_NODES     = N - 1
	SATELLITE_IDS = make([]int, NUM_NODES-1)
	TransmissionDelays = make([][]float64, N)
)


func F() int {
	return f / 10
}

func LoadDynamicTopologyFromFile(filePath string, numNodes int) []*model.SatelliteNode {
	nodes := make([]*model.SatelliteNode, numNodes)
	for i := 0; i < numNodes; i++ {
		nodes[i] = &model.SatelliteNode{
			ID:               i,
			MaxConnections:   6,
			Fragments:        make(map[int]struct{}),
			DynamicNeighbors: make(map[int][]model.TimeInterval),
			Bandwidths:       make(map[int]float64),
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	byteValue, _ := ioutil.ReadAll(file)

	var raw map[string][][]float64
	err = json.Unmarshal(byteValue, &raw)
	if err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	for key, intervals := range raw {
		parts := strings.Split(key, "-")
		if len(parts) != 2 {
			continue
		}
		src, _ := strconv.Atoi(parts[0])
		dst, _ := strconv.Atoi(parts[1])

		for _, interval := range intervals {
			if len(interval) != 2 {
				continue
			}

			ti := model.TimeInterval{Start: interval[0]/100, End: interval[1]/100}

			nodes[src].DynamicNeighbors[dst] = append(nodes[src].DynamicNeighbors[dst], ti)
			nodes[dst].DynamicNeighbors[src] = append(nodes[dst].DynamicNeighbors[src], ti)


			bw := 90 + rand.Float64()*20

			if src == 0 || dst == 0 {
				bw = bw * 0.25
			}

			nodes[src].Bandwidths[dst] = bw
			nodes[dst].Bandwidths[src] = bw
		}
	}

	return nodes
}


func init() {
	for i := 0; i < NUM_NODES-1; i++ {
		SATELLITE_IDS[i] = i + 1
	}

	for i := 0; i < N; i++ {
		row := make([]float64, N)
		for j := 0; j < N; j++ {
			if i == j {
				row[j] = 0.0
			} else {
				row[j] = 0.01
			}
		}
		TransmissionDelays[i] = row
	}

	rand.Seed(42)

	for i := 1; i < NUM_NODES; i++ {
		TransmissionDelays[0][i] = 0.1
		TransmissionDelays[i][0] = 0.1
	}
}
