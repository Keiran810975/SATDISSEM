package algorithms

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sate/globals"
	"sate/model"
)

// 消融实验配置
type AblationConfig struct {
	Name                string
	EnableLoadBalancing bool
	EnableConflictAvoid bool
	EnableMissingFreq   bool
	InjectionAlpha      float64 // 注入权重参数 α
}

var (
	simStartAblation = time.Now()
	lockAblation     sync.Mutex
)

func currentSimulationTimeAblation() float64 {
	return time.Since(simStartAblation).Seconds()
}

// computeHoldingRatioAblation 计算 Regional Feedback：局部持有率
func computeHoldingRatioAblation(station *model.SatelliteNode, fragment int, allNodes []*model.SatelliteNode) float64 {
	now := currentSimulationTimeAblation()
	total, have := 0, 0
	for _, node := range allNodes {
		if node.ID == station.ID || !station.IsConnected(node.ID, now) {
			continue
		}
		lockAblation.Lock()
		_, ok := node.Fragments[fragment]
		lockAblation.Unlock()
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

// computeGlobalSeedingScoreAblation 计算 Global Seeding：全局缺失频率归一化分数
func computeGlobalSeedingScoreAblation(fragment int, globalMissingFreq map[int]int, totalNodes int) float64 {
	missingCount := globalMissingFreq[fragment]
	if totalNodes == 0 {
		return 0.0
	}
	return float64(missingCount) / float64(totalNodes)
}

// computeGlobalMissingFrequencyAblation 计算全局缺失频率
func computeGlobalMissingFrequencyAblation(nodes []*model.SatelliteNode) map[int]int {
	missing := make(map[int]int)
	for _, node := range nodes {
		for i := 0; i < globals.F(); i++ {
			lockAblation.Lock()
			_, ok := node.Fragments[i]
			lockAblation.Unlock()
			if !ok {
				missing[i]++
			}
		}
	}
	return missing
}

// transmitFromStationMultipleAblation 基站传输函数（消融版本）
func transmitFromStationMultipleAblation(station, receiver *model.SatelliteNode, fragCounts map[int]int, nodes []*model.SatelliteNode, globalMissingFreq map[int]int, config AblationConfig) bool {
	now := currentSimulationTimeAblation()
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
			lockAblation.Lock()
			diff := model.FragmentDiff(station.Fragments, receiver.Fragments)
			lockAblation.Unlock()

			fragList := make([]int, 0, len(diff))
			for f := range diff {
				fragList = append(fragList, f)
			}

			// 负载均衡排序（根据配置）
			if config.EnableLoadBalancing {
				totalNodes := len(nodes) - 1
				sort.Slice(fragList, func(i, j int) bool {
					fragI := fragList[i]
					fragJ := fragList[j]

					// Global Seeding
					globalScoreI := computeGlobalSeedingScoreAblation(fragI, globalMissingFreq, totalNodes)
					globalScoreJ := computeGlobalSeedingScoreAblation(fragJ, globalMissingFreq, totalNodes)

					// Regional Feedback
					holdingRatioI := computeHoldingRatioAblation(station, fragI, nodes)
					holdingRatioJ := computeHoldingRatioAblation(station, fragJ, nodes)
					regionalScoreI := 1.0 - holdingRatioI
					regionalScoreJ := 1.0 - holdingRatioJ

					// 综合分数
					scoreI := config.InjectionAlpha*globalScoreI + (1.0-config.InjectionAlpha)*regionalScoreI
					scoreJ := config.InjectionAlpha*globalScoreJ + (1.0-config.InjectionAlpha)*regionalScoreJ

					return scoreI > scoreJ
				})
			} else {
				// 无负载均衡：简单排序
				sort.Ints(fragList)
			}

			for _, frag := range fragList {
				delay := alpha + globals.FRAGMENT_SIZE_MB/bw
				if delay > rem {
					break
				}

				lockAblation.Lock()
				_, alreadyHave := receiver.Fragments[frag]
				if !alreadyHave {
					receiver.Fragments[frag] = struct{}{}
					fragCounts[frag]++
					lockAblation.Unlock()
					atomic.AddInt64(&ConflictAwareCount, 1)
					atomic.AddInt64(&ConflictAwareCount2, 3)
				} else {
					lockAblation.Unlock()
					atomic.AddInt64(&ConflictAwareCount2, 3)
					if config.EnableConflictAvoid {
						// 冲突避免：跳过已拥有的分片
						atomic.AddInt64(&ConflictAwareCount, 3)
						continue
					} else {
						// 无冲突避免：继续传输（虽然已拥有，但可能用于其他目的）
						atomic.AddInt64(&ConflictAwareCount, 1)
					}
				}

				fmt.Printf("[%.2fs] 🚀 Station→%d sending %d\n", currentSimulationTimeAblation(), receiver.ID, frag)
				time.Sleep(time.Duration(delay * float64(time.Second)))
				rem -= delay
			}
			break
		}
	}
	return true
}

// transmitSatelliteFragmentsMultipleAblation 卫星间传输函数（消融版本）
func transmitSatelliteFragmentsMultipleAblation(sender, receiver *model.SatelliteNode, missingFreq map[int]int, config AblationConfig) bool {
	now := currentSimulationTimeAblation()
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

			lockAblation.Lock()
			diff := model.FragmentDiff(sender.Fragments, receiver.Fragments)
			lockAblation.Unlock()

			fragList := make([]int, 0, len(diff))
			for f := range diff {
				fragList = append(fragList, f)
			}

			// 缺失频率排序（根据配置）
			if config.EnableMissingFreq {
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

				lockAblation.Lock()
				_, alreadyHave := receiver.Fragments[frag]
				if !alreadyHave {
					receiver.Fragments[frag] = struct{}{}
					lockAblation.Unlock()
					atomic.AddInt64(&ConflictAwareCount, 1)
					atomic.AddInt64(&ConflictAwareCount2, 3)
					fmt.Printf("[%.2fs] 🛰️ %d→%d stable:%t sending %d (missing:%d)\n",
						currentSimulationTimeAblation(), sender.ID, receiver.ID, isStable, frag, missingFreq[frag])
				} else {
					lockAblation.Unlock()
					atomic.AddInt64(&ConflictAwareCount2, 3)
					if config.EnableConflictAvoid {
						atomic.AddInt64(&ConflictAwareCount, 3)
						continue
					} else {
						atomic.AddInt64(&ConflictAwareCount, 1)
					}
				}

				time.Sleep(time.Duration(delay * float64(time.Second)))
				rem -= delay
			}
			break
		}
	}
	return true
}

// experimentNodeThreadAblation 节点线程（消融版本）
func experimentNodeThreadAblation(node *model.SatelliteNode, nodes []*model.SatelliteNode, fragCounts map[int]int, wg *sync.WaitGroup, config AblationConfig) {
	defer wg.Done()
	sem := make(chan struct{}, MAX_PARALLEL_TRANS)

	for {
		now := currentSimulationTimeAblation()
		lockAblation.Lock()
		allDone := true
		for _, n := range nodes {
			if len(n.Fragments) < globals.F() {
				allDone = false
				break
			}
		}
		lockAblation.Unlock()
		if allDone {
			break
		}

		missingFreq := computeGlobalMissingFrequencyAblation(nodes)

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
					transmitFromStationMultipleAblation(node, nei, fragCounts, nodes, missingFreq, config)
				} else {
					transmitSatelliteFragmentsMultipleAblation(node, nei, missingFreq, config)
				}
			}(neighbor)
		}
		localWg.Wait()
		time.Sleep(10 * time.Millisecond)
	}
}

// RunSimulationAblation 运行消融实验
func RunSimulationAblation(nodes []*model.SatelliteNode, config AblationConfig) {
	simStartAblation = time.Now()

	fragCounts := make(map[int]int)
	for i := 0; i < globals.F(); i++ {
		fragCounts[i] = 0
	}
	nodes[0].Fragments = make(map[int]struct{})
	for i := 0; i < globals.F(); i++ {
		nodes[0].Fragments[i] = struct{}{}
	}

	fmt.Printf("\n🔬 ========== Ablation Study: %s ==========\n", config.Name)
	fmt.Printf("  - Load Balancing: %v\n", config.EnableLoadBalancing)
	fmt.Printf("  - Conflict Avoidance: %v\n", config.EnableConflictAvoid)
	fmt.Printf("  - Missing Frequency Priority: %v\n", config.EnableMissingFreq)
	fmt.Printf("  - Injection weight α: %.2f\n", config.InjectionAlpha)
	fmt.Printf("  - Global Seeding weight: %.2f, Regional Feedback weight: %.2f\n",
		config.InjectionAlpha, 1.0-config.InjectionAlpha)

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go experimentNodeThreadAblation(node, nodes, fragCounts, &wg, config)
	}
	wg.Wait()

	fmt.Printf("\n✅ Ablation study [%s] completed, total time: %.2f seconds\n", config.Name, currentSimulationTimeAblation())
}

// RunAblationStudy 运行完整的消融实验研究
func RunAblationStudy(nodes []*model.SatelliteNode) {
	separator := strings.Repeat("=", 60)
	fmt.Printf("\n%s\n", separator)
	fmt.Printf("🔬 Starting ablation study\n")
	fmt.Printf("%s\n", separator)

	// 定义消融实验配置
	configs := []AblationConfig{
		{
			Name:                "完整版本 (Full)",
			EnableLoadBalancing: true,
			EnableConflictAvoid: true,
			EnableMissingFreq:   true,
			InjectionAlpha:      INJECTION_ALPHA,
		},
		{
			Name:                "无负载均衡 (No Load Balancing)",
			EnableLoadBalancing: false,
			EnableConflictAvoid: true,
			EnableMissingFreq:   true,
			InjectionAlpha:      INJECTION_ALPHA,
		},
		{
			Name:                "无冲突避免 (No Conflict Avoidance)",
			EnableLoadBalancing: true,
			EnableConflictAvoid: false,
			EnableMissingFreq:   true,
			InjectionAlpha:      INJECTION_ALPHA,
		},
		{
			Name:                "无缺失频率优先 (No Missing Frequency)",
			EnableLoadBalancing: true,
			EnableConflictAvoid: true,
			EnableMissingFreq:   false,
			InjectionAlpha:      INJECTION_ALPHA,
		},
	}

	results := make(map[string]float64)

	// 运行每个消融实验
	for i, config := range configs {
		fmt.Printf("\n%s\n", separator)
		fmt.Printf("Experiment %d/%d: %s\n", i+1, len(configs), config.Name)
		fmt.Printf("%s\n", separator)

		// 重置节点状态
		for j := range nodes {
			nodes[j].Fragments = make(map[int]struct{})
			for k := 0; k < globals.F(); k++ {
				if j == 0 {
					nodes[j].Fragments[k] = struct{}{}
				}
			}
		}

		// 运行实验
		startTime := time.Now()
		RunSimulationAblation(nodes, config)
		duration := time.Since(startTime).Seconds()
		results[config.Name] = duration

		fmt.Printf("⏱️  Experiment duration: %.2f seconds\n", duration)
	}

	// 输出汇总结果
	fmt.Printf("\n%s\n", separator)
	fmt.Printf("📊 Ablation study results summary\n")
	fmt.Printf("%s\n", separator)
	fmt.Printf("%-40s %12s %12s\n", "Configuration", "Time(s)", "vs Full")
	fmt.Printf("%-40s %12s %12s\n", strings.Repeat("-", 40), strings.Repeat("-", 12), strings.Repeat("-", 12))

	baseline := results["完整版本 (Full)"]
	for _, config := range configs {
		duration := results[config.Name]
		relative := (duration - baseline) / baseline * 100
		fmt.Printf("%-40s %12.2f %11.1f%%\n", config.Name, duration, relative)
	}

	fmt.Printf("\n%s\n", separator)
}
