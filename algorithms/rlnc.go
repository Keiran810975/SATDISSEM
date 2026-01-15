package algorithms

import (
	"fmt"
	"sync"
	"time"

	"sate/globals"
	"sate/model"
)

const (
	MAX_PARALLEL_TRANS_RLNC = 6
)

var (
	rlncStart = time.Now()
	lockRLNC  sync.Mutex
)

func currentSimulationTimeRLNC() float64 {
	return time.Since(rlncStart).Seconds()
}

func transmitRLNC(sender, receiver *model.SatelliteNode, ranks map[int]int) {
	now := currentSimulationTimeRLNC()
	if !sender.IsConnected(receiver.ID, now) {
		return
	}

	alpha := globals.TransmissionDelays[sender.ID][receiver.ID]
	bw := sender.GetBandwidth(receiver.ID)
	if bw <= 0 {
		return
	}

	for _, iv := range sender.DynamicNeighbors[receiver.ID] {
		if iv.Start <= now && now < iv.End {
			rem := iv.End - now
			delay := alpha + globals.FRAGMENT_SIZE_MB/bw

			for rem >= delay {

				lockRLNC.Lock()
				sr := ranks[sender.ID]
				rr := ranks[receiver.ID]
				innovative := rr < sr && rr < globals.F()
				lockRLNC.Unlock()

				if !innovative {
					break
				}

				computeDelay := delay * 0.05
				time.Sleep(time.Duration(computeDelay * float64(time.Second)))

				fmt.Printf("[%.2fs] 🔗 RLNC %d→%d sending encoded packet (innov:%t, recvRank:%d/%d)\n",
					currentSimulationTimeRLNC(), sender.ID, receiver.ID, innovative, rr, globals.F())

				time.Sleep(time.Duration(delay * float64(time.Second)))
				rem -= (delay + computeDelay)

				if innovative {
					lockRLNC.Lock()

					if ranks[receiver.ID] < sr && ranks[receiver.ID] < globals.F() {
						ranks[receiver.ID]++
						if ranks[receiver.ID] == globals.F() {
							for i := 0; i < globals.F(); i++ {
								receiver.Fragments[i] = struct{}{}
							}
							fmt.Printf("[%.2fs] ✅ RLNC decoding completed: node %d obtained all fragments\n",
								currentSimulationTimeRLNC(), receiver.ID)
						}
					}
					lockRLNC.Unlock()
				}
			}
			break
		}
	}
}

func rlncNodeThread(node *model.SatelliteNode, nodes []*model.SatelliteNode, ranks map[int]int, wg *sync.WaitGroup) {
	defer wg.Done()
	sem := make(chan struct{}, MAX_PARALLEL_TRANS_RLNC)

	for {

		lockRLNC.Lock()
		allDone := true
		for _, n := range nodes {
			if len(n.Fragments) < globals.F() {
				allDone = false
				break
			}
		}
		lockRLNC.Unlock()
		if allDone {
			break
		}

		now := currentSimulationTimeRLNC()
		var localWg sync.WaitGroup

		for nbID := range node.DynamicNeighbors {
			if nbID == node.ID {
				continue
			}
			neighbor := nodes[nbID]
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
				transmitRLNC(node, nei, ranks)
			}(neighbor)
		}

		localWg.Wait()
		time.Sleep(10 * time.Millisecond)
	}
}

func RunSimulationRLNC(nodes []*model.SatelliteNode) {
	rlncStart = time.Now()

	ranks := make(map[int]int, len(nodes))

	for i := range nodes {
		lockRLNC.Lock()
		if i == 0 {
			if nodes[0].Fragments == nil {
				nodes[0].Fragments = make(map[int]struct{})
			}
			for f := 0; f < globals.F(); f++ {
				nodes[0].Fragments[f] = struct{}{}
			}
			ranks[0] = globals.F()
		} else {
			ranks[i] = len(nodes[i].Fragments)
		}
		lockRLNC.Unlock()
	}

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go rlncNodeThread(node, nodes, ranks, &wg)
	}
	wg.Wait()

	fmt.Printf("\nRLNC control group completed, total time: %.2f seconds\n", currentSimulationTimeRLNC())
}
