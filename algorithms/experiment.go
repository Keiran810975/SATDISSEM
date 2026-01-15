package algorithms

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"sate/globals"
	"sate/model"
)

const (
	STABLE_THRESHOLD   = 3.0
	MAX_PARALLEL_TRANS = 6

	INJECTION_ALPHA = 0.6 
)

var (
	simStart            = time.Now()
	lockFlooding        sync.Mutex
	lockExperiment      sync.Mutex

	EnableLoadBalancing     = true
	EnableMissingFrequency  = true
	EnableConflictAvoidance = true
)

func currentSimulationTime() float64 {
	return time.Since(simStart).Seconds()
}

func computeHoldingRatio(station *model.SatelliteNode, fragment int, allNodes []*model.SatelliteNode) float64 {
	now := currentSimulationTime()
	total, have := 0, 0
	for _, node := range allNodes {
		if node.ID == station.ID || !station.IsConnected(node.ID, now) {
			continue
		}
		lockExperiment.Lock()
		_, ok := node.Fragments[fragment]
		lockExperiment.Unlock()
		if ok {
			have++
		}
		total++
	}
	if total == 0 {
		return 1.0
	}
	return float64(have) / float64(total)
}

func computeGlobalSeedingScore(fragment int, globalMissingFreq map[int]int, totalNodes int) float64 {
	missingCount := globalMissingFreq[fragment]
	if totalNodes == 0 {
		return 0.0
	}

	return float64(missingCount) / float64(totalNodes)
}

func computeGlobalMissingFrequency(nodes []*model.SatelliteNode) map[int]int {
	missing := make(map[int]int)
	for _, node := range nodes {
		for i := 0; i < globals.F(); i++ {
			lockExperiment.Lock()
			_, ok := node.Fragments[i]
			lockExperiment.Unlock()
			if !ok {
				missing[i]++
			}
		}
	}
	return missing
}

func transmitFromStationMultiple(station, receiver *model.SatelliteNode, fragCounts map[int]int, nodes []*model.SatelliteNode, globalMissingFreq map[int]int) bool {
	now := currentSimulationTime()
	if !station.IsConnected(receiver.ID, now) {
		return false
	}

	alpha := globals.TransmissionDelays[station.ID][receiver.ID]
	bw := station.GetBandwidth(receiver.ID)
	if bw <= 0 {
		return false
	}

	for _, iv := range station.DynamicNeighbors[receiver.ID] {
		if iv.Start <= now && now < iv.End {
			rem := iv.End - now
			lockExperiment.Lock()
			diff := model.FragmentDiff(station.Fragments, receiver.Fragments)
			lockExperiment.Unlock()

			fragList := make([]int, 0, len(diff))
			for f := range diff {
				fragList = append(fragList, f)
			}

			if EnableLoadBalancing {
				totalNodes := len(nodes) - 1
				sort.Slice(fragList, func(i, j int) bool {
					fragI := fragList[i]
					fragJ := fragList[j]

					globalScoreI := computeGlobalSeedingScore(fragI, globalMissingFreq, totalNodes)
					globalScoreJ := computeGlobalSeedingScore(fragJ, globalMissingFreq, totalNodes)


					holdingRatioI := computeHoldingRatio(station, fragI, nodes)
					holdingRatioJ := computeHoldingRatio(station, fragJ, nodes)
					regionalScoreI := 1.0 - holdingRatioI 
					regionalScoreJ := 1.0 - holdingRatioJ

					scoreI := INJECTION_ALPHA*globalScoreI + (1.0-INJECTION_ALPHA)*regionalScoreI
					scoreJ := INJECTION_ALPHA*globalScoreJ + (1.0-INJECTION_ALPHA)*regionalScoreJ
					return scoreI > scoreJ
				})
			}

			for _, frag := range fragList {
				delay := alpha + globals.FRAGMENT_SIZE_MB/bw
				if delay > rem {
					break
				}

				lockExperiment.Lock()
				_, alreadyHave := receiver.Fragments[frag]
				if !alreadyHave {
					// 成功传输：接收方没有该分片
					receiver.Fragments[frag] = struct{}{}
					fragCounts[frag]++
					lockExperiment.Unlock()
				
				} else {
					// 接收方已拥有该分片
					lockExperiment.Unlock()
				}

				fmt.Printf("[%.2fs] 🚀 Station→%d sending %d\n", currentSimulationTime(), receiver.ID, frag)
				time.Sleep(time.Duration(delay * float64(time.Second)))
				rem -= delay
			}
			break
		}
	}
	return true
}

func transmitSatelliteFragmentsMultiple(sender, receiver *model.SatelliteNode, missingFreq map[int]int) bool {
	now := currentSimulationTime()
	if !sender.IsConnected(receiver.ID, now) {
		return false
	}

	alpha := globals.TransmissionDelays[sender.ID][receiver.ID]
	bw := sender.GetBandwidth(receiver.ID)
	if bw <= 0 {
		return false
	}

	for _, iv := range sender.DynamicNeighbors[receiver.ID] {
		if iv.Start <= now && now < iv.End {
			rem := iv.End - now
			isStable := iv.End-iv.Start >= STABLE_THRESHOLD

			lockExperiment.Lock()
			diff := model.FragmentDiff(sender.Fragments, receiver.Fragments)
			lockExperiment.Unlock()

			fragList := make([]int, 0, len(diff))
			for f := range diff {
				fragList = append(fragList, f)
			}

			if EnableMissingFrequency {
				sort.Slice(fragList, func(i, j int) bool {
					return missingFreq[fragList[i]] > missingFreq[fragList[j]]
				})
			}

			for _, frag := range fragList {
				delay := alpha + globals.FRAGMENT_SIZE_MB/bw
				if delay > rem {
					break
				}

				lockExperiment.Lock()
				_, alreadyHave := receiver.Fragments[frag]
				if !alreadyHave {

					receiver.Fragments[frag] = struct{}{}
					lockExperiment.Unlock()
				} else {

					lockExperiment.Unlock()
				}

				time.Sleep(time.Duration(delay * float64(time.Second)))
				fmt.Printf("[%.2fs] 🛰️ %d→%d stable:%t sending %d (missing:%d)\n",
					currentSimulationTime(), sender.ID, receiver.ID, isStable, frag, missingFreq[frag])
				rem -= delay
			}
			break
		}
	}
	return true
}

func experimentNodeThread(node *model.SatelliteNode, nodes []*model.SatelliteNode, fragCounts map[int]int, wg *sync.WaitGroup) {
	defer wg.Done()
	sem := make(chan struct{}, MAX_PARALLEL_TRANS)

	for {
		now := currentSimulationTime()
		lockExperiment.Lock()
		allDone := true
		for _, n := range nodes {
			if len(n.Fragments) < globals.F() {
				allDone = false
				break
			}
		}
		lockExperiment.Unlock()
		if allDone {
			break
		}

		missingFreq := computeGlobalMissingFrequency(nodes)

		var localWg sync.WaitGroup
		for id := range node.DynamicNeighbors {
			if id == node.ID {
				continue
			}
			neighbor := nodes[id]
			if !node.IsConnected(neighbor.ID, now) {
				continue
			}

			sem <- struct{}{}
			localWg.Add(1)
			go func(nei *model.SatelliteNode) {
				defer func() {
					<-sem
					localWg.Done()
				}()
				if node.ID == 0 {
					transmitFromStationMultiple(node, nei, fragCounts, nodes, missingFreq)
				} else {
					transmitSatelliteFragmentsMultiple(node, nei, missingFreq)
				}
			}(neighbor)
		}
		localWg.Wait()
		time.Sleep(10 * time.Millisecond)
	}
}

func RunSimulationExperiment(nodes []*model.SatelliteNode) {
	simStart = time.Now()

	fragCounts := make(map[int]int)
	for i := 0; i < globals.F(); i++ {
		fragCounts[i] = 0
	}
	nodes[0].Fragments = make(map[int]struct{})
	for i := 0; i < globals.F(); i++ {
		nodes[0].Fragments[i] = struct{}{}
	}

	// Output algorithm parameters
	totalNodes := len(nodes) - 1
	fmt.Printf("\n🔬 Experiment group algorithm parameters:\n")
	fmt.Printf("  - Number of nodes: %d\n", totalNodes)
	fmt.Printf("  - Injection weight α: %.2f (Global Seeding weight)\n", INJECTION_ALPHA)
	fmt.Printf("  - Regional Feedback weight: %.2f\n", 1.0-INJECTION_ALPHA)
	if totalNodes < 500 {
		fmt.Printf("  - Recommended α range: 0.4-0.5 (small-scale network)\n")
	} else if totalNodes < 1000 {
		fmt.Printf("  - Recommended α range: 0.5-0.6 (medium-scale network)\n")
	} else {
		fmt.Printf("  - Recommended α range: 0.6-0.7 (large-scale network)\n")
	}
	fmt.Printf("  - Load balancing: %v\n", EnableLoadBalancing)
	fmt.Printf("  - Missing frequency priority: %v\n", EnableMissingFrequency)
	fmt.Printf("  - Conflict avoidance: %v\n", EnableConflictAvoidance)

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go experimentNodeThread(node, nodes, fragCounts, &wg)
	}
	wg.Wait()

	fmt.Printf("\nExperiment group completed, total time: %.2f seconds\n", currentSimulationTime())
}
