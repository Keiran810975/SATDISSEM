package algorithms

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"sate/globals"
	"sate/model"
)

var (
	F     = globals.F()
	BETA  = 1.0
	start = time.Now()
	lock  sync.Mutex
)

//func currentSimulationTime() float64 {
//	return time.Since(start).Seconds()
//}

// 模拟实验组计算耗时
func simulateComputeDelay(sender, receiver *model.SatelliteNode, allNodes []*model.SatelliteNode) {
	lock.Lock()
	defer lock.Unlock()

	diff := make([]int, 0)
	for f := 0; f < F; f++ {
		_, has := sender.Fragments[f]
		_, recvHas := receiver.Fragments[f]
		if has && !recvHas {
			diff = append(diff, f)
		}
	}

	missing := make(map[int]int)
	for _, n := range allNodes {
		for i := 0; i < F; i++ {
			if _, ok := n.Fragments[i]; !ok {
				missing[i]++
			}
		}
	}

	sort.Slice(diff, func(i, j int) bool {
		return missing[diff[i]] > missing[diff[j]]
	})

	for _, frag := range diff {
		total := 0
		have := 0
		for _, n := range allNodes {
			if !sender.IsConnected(n.ID, currentSimulationTime()) {
				continue
			}
			total++
			if _, ok := n.Fragments[frag]; ok {
				have++
			}
		}
		_ = float64(have) / float64(total+1)
	}
}

func transmitFragmentsMultiple(sender, receiver *model.SatelliteNode, allNodes []*model.SatelliteNode) {
	now := currentSimulationTime()

	if !sender.IsConnected(receiver.ID, now) {
		return
	}

	alpha := globals.TransmissionDelays[sender.ID][receiver.ID]
	bandwidth := sender.GetBandwidth(receiver.ID)
	if bandwidth <= 0 {
		return
	}

	delay := alpha + globals.FRAGMENT_SIZE_MB/bandwidth
	intervals := sender.DynamicNeighbors[receiver.ID]

	for _, iv := range intervals {
		if iv.Start <= now && now < iv.End {
			remainingTime := iv.End - now

			fragList := []int{}
			lock.Lock()
			for f := range sender.Fragments {
				if _, ok := receiver.Fragments[f]; !ok {
					fragList = append(fragList, f)
				}
			}
			lock.Unlock()

			sort.Ints(fragList)

			for _, frag := range fragList {
				lock.Lock()
				if _, ok := receiver.Fragments[frag]; ok {
					lock.Unlock()
					continue
				}
				lock.Unlock()

				if delay > remainingTime {
					break
				}

				// 模拟算法计算延迟
				simulateComputeDelay(sender, receiver, allNodes)

				lock.Lock()
				if _, ok := receiver.Fragments[frag]; ok {
					lock.Unlock()
					continue
				}
				//time.Sleep(time.Duration(delay * float64(time.Second)))
				fmt.Println(delay)
				fmt.Printf("[%.2fs] 🚀 Satellite %d sending fragment %d to %d\n", currentSimulationTime(), sender.ID, receiver.ID, frag)
				lock.Unlock()

				time.Sleep(time.Duration(delay * float64(time.Second)))

				lock.Lock()
				receiver.Fragments[frag] = struct{}{}
				fmt.Printf("[%.2fs] ✅ Satellite %d successfully received fragment %d from satellite %d\n", currentSimulationTime(), receiver.ID, frag, sender.ID)

				owned := []int{}
				for f := range receiver.Fragments {
					owned = append(owned, f)
				}
				sort.Ints(owned)
				//fmt.Printf("[%.2fs] 📦 当前 %d 拥有分片：%v\n", currentSimulationTime(), receiver.ID, owned)
				lock.Unlock()

				now += delay
				remainingTime -= delay
			}
			break
		}
	}
}

func floodingNodeThread(node *model.SatelliteNode, nodes []*model.SatelliteNode, wg *sync.WaitGroup) {
	defer wg.Done()

	sem := make(chan struct{}, 6)

	for {
		now := currentSimulationTime()
		lock.Lock()
		done := true
		for _, n := range nodes {
			if len(n.Fragments) < F {
				done = false
				break
			}
		}
		lock.Unlock()
		if done {
			break
		}

		var localWg sync.WaitGroup
		for nbID := range node.DynamicNeighbors {
			if node.ID == nbID {
				continue
			}
			receiver := nodes[nbID]
			sem <- struct{}{}
			localWg.Add(1)

			go func(r *model.SatelliteNode) {
				defer func() {
					<-sem
					localWg.Done()
				}()
				if node.IsConnected(r.ID, now) {
					transmitFragmentsMultiple(node, r, nodes)
				}
			}(receiver)
		}
		localWg.Wait()
		time.Sleep(10 * time.Millisecond)
	}
}

func RunSimulationFlooding(nodes []*model.SatelliteNode) {
	for i := 0; i < F; i++ {
		nodes[0].Fragments[i] = struct{}{}
	}

	//go func() {
	//	for {
	//		time.Sleep(100 * time.Millisecond)
	//		if currentSimulationTime() >= 115.0 {
	//			lock.Lock()
	//			fmt.Println("\n⏱️ 超时终止：模拟已运行超过 115 秒")
	//			for _, n := range nodes {
	//				owned := []int{}
	//				for f := range n.Fragments {
	//					owned = append(owned, f)
	//				}
	//				sort.Ints(owned)
	//				fmt.Printf("🛰️ 卫星 %d 拥有分片: %v\n", n.ID, owned)
	//			}
	//			lock.Unlock()
	//			panic("⛔ 模拟中止：超时115秒")
	//		}
	//	}
	//}()

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go floodingNodeThread(node, nodes, &wg)
	}
	wg.Wait()

	fmt.Printf("\n✅ All satellites completed flooding transmission, total time: %.2f seconds\n", currentSimulationTime())
}
