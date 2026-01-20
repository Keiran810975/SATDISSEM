package algorithms

import (
	"fmt"
	"os"
	"sate/globals"
	"sate/model"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	STABLE_THRESHOLD   = 3.0
	MAX_PARALLEL_TRANS = 6

	INJECTION_ALPHA = 0.40
)

var (
	simStart       = time.Now()
	lockFlooding   sync.Mutex
	lockExperiment sync.Mutex
	receiverBusyMu sync.Mutex
	receiverBusy   []int32

	EnableLoadBalancing     = true
	EnableMissingFrequency  = true
	EnableConflictAvoidance = true
)

func resetReceiverBusy(totalNodes int) {
	receiverBusyMu.Lock()
	defer receiverBusyMu.Unlock()

	if totalNodes <= 0 {
		receiverBusy = nil
		return
	}
	if receiverBusy == nil || len(receiverBusy) != totalNodes {
		receiverBusy = make([]int32, totalNodes)
		return
	}
	for i := range receiverBusy {
		atomic.StoreInt32(&receiverBusy[i], 0)
	}
}

func tryAcquireReceiver(receiverID int) bool {
	if !EnableConflictAvoidance {
		return true
	}
	if receiverID < 0 || receiverID >= len(receiverBusy) {
		return true
	}
	return atomic.CompareAndSwapInt32(&receiverBusy[receiverID], 0, 1)
}

func releaseReceiver(receiverID int) {
	if !EnableConflictAvoidance {
		return
	}
	if receiverID < 0 || receiverID >= len(receiverBusy) {
		return
	}
	atomic.StoreInt32(&receiverBusy[receiverID], 0)
}

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
			} else {
				sort.Ints(fragList)
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
					fragCounts[frag]++
					lockExperiment.Unlock()
				} else {
					lockExperiment.Unlock()
				}

				fmt.Printf("[%.2fs] Station→%d sending %d\n", currentSimulationTime(), receiver.ID, frag)
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
			} else {
				sort.Ints(fragList)
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
					continue
				}

				time.Sleep(time.Duration(delay * float64(time.Second)))
				fmt.Printf("[%.2fs] %d→%d stable:%t sending %d (missing:%d)\n",
					currentSimulationTime(), sender.ID, receiver.ID, isStable, frag, missingFreq[frag])
				rem -= delay
			}
			break
		}
	}
	return true
}

var (
	cachedMissingFreq     map[int]int
	cachedMissingFreqLock sync.RWMutex
	lastMissingFreqUpdate time.Time
)

func getCachedMissingFreq(nodes []*model.SatelliteNode) map[int]int {
	cachedMissingFreqLock.RLock()
	if cachedMissingFreq != nil && time.Since(lastMissingFreqUpdate) < 100*time.Millisecond {
		result := cachedMissingFreq
		cachedMissingFreqLock.RUnlock()
		return result
	}
	cachedMissingFreqLock.RUnlock()

	cachedMissingFreqLock.Lock()
	defer cachedMissingFreqLock.Unlock()

	if cachedMissingFreq != nil && time.Since(lastMissingFreqUpdate) < 100*time.Millisecond {
		return cachedMissingFreq
	}

	cachedMissingFreq = computeGlobalMissingFrequency(nodes)
	lastMissingFreqUpdate = time.Now()
	return cachedMissingFreq
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

		missingFreq := getCachedMissingFreq(nodes)

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

				if !tryAcquireReceiver(nei.ID) {
					return
				}
				defer releaseReceiver(nei.ID)

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
	resetReceiverBusy(len(nodes))

	cachedMissingFreqLock.Lock()
	cachedMissingFreq = nil
	lastMissingFreqUpdate = time.Time{}
	cachedMissingFreqLock.Unlock()

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

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go experimentNodeThread(node, nodes, fragCounts, &wg)
	}
	wg.Wait()

	fmt.Printf("\nExperiment group completed, total time: %.2f seconds\n", currentSimulationTime())

	finalMissing := computeGlobalMissingFrequency(nodes)
	totalMissingCount := 0
	for _, count := range finalMissing {
		totalMissingCount += count
	}
	if totalMissingCount == 0 {
		fmt.Println("Final Validation Passed: All fragments are fully distributed (Total Missing: 0).")
	} else {
		fmt.Printf("Final Validation Failed: Total Missing Fragments: %d\n", totalMissingCount)
	}

	file, err := os.OpenFile(
		"output.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	fmt.Fprintf(
		file,
		"Experiment sate_num=%d , F=%d ,F_Size=%.2fMB ,ALPHA=%.2f, time:%.2f seconds\n",
		len(nodes)-1,
		globals.F(),
		globals.FRAGMENT_SIZE_MB,
		INJECTION_ALPHA,
		currentSimulationTime(),
	)
}
