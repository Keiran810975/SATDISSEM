package algorithms

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"sate/globals"
	"sate/model"
)

type SimpleDFedSatNode struct {
	*model.SatelliteNode
	Fragments      map[int]bool
	PlaneID        int
	IntraNeighbors []int
	InterNeighbors []int
}

var (
	dfedSatSimpleStart time.Time
)

func currentSimulationTimeDFedSat() float64 {
	return time.Since(dfedSatSimpleStart).Seconds()
}

func RunSimulationDFedSat(nodes []*model.SatelliteNode) {
	dfedSatSimpleStart = time.Now()
	rand.Seed(42)

	// 初始化节点
	dfedNodes := make([]*SimpleDFedSatNode, len(nodes))
	for i, node := range nodes {
		dfedNodes[i] = &SimpleDFedSatNode{
			SatelliteNode: node,
			Fragments:     make(map[int]bool),
			PlaneID:       -1,
		}
	}

	numPlanes := 4
	totalSats := len(dfedNodes) - 1
	satsPerPlane := (totalSats + numPlanes - 1) / numPlanes

	dfedNodes[0].PlaneID = -1
	for i := 1; i < len(dfedNodes); i++ {
		dfedNodes[i].PlaneID = (i - 1) / satsPerPlane
		if dfedNodes[i].PlaneID >= numPlanes {
			dfedNodes[i].PlaneID = numPlanes - 1
		}
	}

	for i, node := range dfedNodes {
		intra := make([]int, 0)
		inter := make([]int, 0)
		for j, other := range dfedNodes {
			if i == j {
				continue
			}
			if node.PlaneID == other.PlaneID && node.PlaneID >= 0 {
				intra = append(intra, j)
			} else if node.PlaneID >= 0 && other.PlaneID >= 0 {
				inter = append(inter, j)
			}
		}
		node.IntraNeighbors = intra
		node.InterNeighbors = inter
	}

	for i := 0; i < globals.F(); i++ {
		dfedNodes[0].Fragments[i] = true
	}

	fmt.Printf("\n🛰️  Starting DFedSat Simplified: N=%d, F=%d, Planes=%d\n", len(dfedNodes)-1, globals.F(), numPlanes)
	fmt.Println("  Features: Two-level diffusion (intra-plane strong + inter-plane random gossip), based on Flooding async framework")

	var wg sync.WaitGroup
	lock := sync.Mutex{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		const sleepInterval = 10 * time.Millisecond
		station := dfedNodes[0]

		for {
			now := currentSimulationTimeDFedSat()

			lock.Lock()
			allDone := true
			for _, n := range dfedNodes {
				if len(n.Fragments) < globals.F() {
					allDone = false
					break
				}
			}
			lock.Unlock()

			if allDone {
				break
			}

			for neighborID := range station.SatelliteNode.DynamicNeighbors {
				if neighborID == 0 {
					continue
				}
				neighbor := dfedNodes[neighborID]
				if station.SatelliteNode.IsConnected(neighbor.SatelliteNode.ID, now) {
					bw := station.SatelliteNode.GetBandwidth(neighbor.SatelliteNode.ID)
					if bw > 0 {
						alpha := globals.TransmissionDelays[station.SatelliteNode.ID][neighbor.SatelliteNode.ID]

						for _, iv := range station.SatelliteNode.DynamicNeighbors[neighbor.SatelliteNode.ID] {
							if iv.Start <= now && now < iv.End {
								remainingTime := iv.End - now

								for fragID := 0; fragID < globals.F(); fragID++ {
									lock.Lock()
									if station.Fragments[fragID] && !neighbor.Fragments[fragID] {
										delay := alpha + float64(globals.FRAGMENT_SIZE_MB)/bw
										if delay > remainingTime {
											lock.Unlock()
											break
										}
										
										fmt.Printf("[%.2fs] 🚀 Station→%d sending fragment %d\n",
											currentSimulationTimeDFedSat(), neighbor.SatelliteNode.ID, fragID)
										lock.Unlock()

										time.Sleep(time.Duration(delay * float64(time.Second)))

										lock.Lock()
								
										if !neighbor.Fragments[fragID] {
											neighbor.Fragments[fragID] = true
											
											fmt.Printf("[%.2fs] ✅ Satellite %d successfully received fragment %d from station\n",
												currentSimulationTimeDFedSat(), neighbor.SatelliteNode.ID, fragID)
										}
										remainingTime -= delay
										lock.Unlock()
									} else {
										lock.Unlock()
									}
								}
								break
							}
						}
					}
				}
			}

			time.Sleep(sleepInterval)
		}
	}()

	for i, dnode := range dfedNodes {
		if dnode.PlaneID >= 0 {
			wg.Add(1)
			go func(idx int, node *SimpleDFedSatNode) {
				defer wg.Done()

				const sleepInterval = 10 * time.Millisecond
				sem := make(chan struct{}, 6)

				for {
					now := currentSimulationTimeDFedSat()

					for _, nbrIdx := range node.IntraNeighbors {
						nbr := dfedNodes[nbrIdx]
						if node.SatelliteNode.IsConnected(nbr.SatelliteNode.ID, now) {
				
							bw := node.SatelliteNode.GetBandwidth(nbr.SatelliteNode.ID)
							if bw > 0 {
								alpha := globals.TransmissionDelays[node.SatelliteNode.ID][nbr.SatelliteNode.ID]

								for _, iv := range node.SatelliteNode.DynamicNeighbors[nbr.SatelliteNode.ID] {
									if iv.Start <= now && now < iv.End {
										remainingTime := iv.End - now

										for fragID := 0; fragID < globals.F(); fragID++ {
											lock.Lock()
											if node.Fragments[fragID] && !nbr.Fragments[fragID] {
												delay := alpha + float64(globals.FRAGMENT_SIZE_MB)/bw
												if delay > remainingTime {
													lock.Unlock()
													break
												}

												fmt.Printf("[%.2fs] 🛰️ DFedSat intra-plane %d→%d sending fragment %d\n",
													currentSimulationTimeDFedSat(), node.SatelliteNode.ID, nbr.SatelliteNode.ID, fragID)
												lock.Unlock()

												time.Sleep(time.Duration(delay * float64(time.Second)))

												lock.Lock()

												if !nbr.Fragments[fragID] {
													nbr.Fragments[fragID] = true

													fmt.Printf("[%.2fs] ✅ DFedSat intra-plane %d successfully received fragment %d from satellite %d\n",
														currentSimulationTimeDFedSat(), nbr.SatelliteNode.ID, fragID, node.SatelliteNode.ID)
												}
												remainingTime -= delay
												lock.Unlock()
											} else {
												lock.Unlock()
											}
										}
										break
									}
								}
							}
						}
					}

					if len(node.InterNeighbors) > 0 && rand.Float64() < 0.5 {
						sem <- struct{}{}
						go func() {
							defer func() { <-sem }()

							nbrIdx := node.InterNeighbors[rand.Intn(len(node.InterNeighbors))]
							nbr := dfedNodes[nbrIdx]
							now := currentSimulationTimeDFedSat()

							if node.SatelliteNode.IsConnected(nbr.SatelliteNode.ID, now) {
								bw := node.SatelliteNode.GetBandwidth(nbr.SatelliteNode.ID)
								if bw > 0 {
									alpha := globals.TransmissionDelays[node.SatelliteNode.ID][nbr.SatelliteNode.ID]

									for _, iv := range node.SatelliteNode.DynamicNeighbors[nbr.SatelliteNode.ID] {
										if iv.Start <= now && now < iv.End {
											remainingTime := iv.End - now

											for fragID := 0; fragID < globals.F(); fragID++ {
												lock.Lock()
												if node.Fragments[fragID] && !nbr.Fragments[fragID] {
													delay := alpha + float64(globals.FRAGMENT_SIZE_MB)/bw
													if delay > remainingTime {
														lock.Unlock()
														break
													}

													fmt.Printf("[%.2fs] 🛰️ DFedSat inter-plane %d→%d sending fragment %d\n",
														currentSimulationTimeDFedSat(), node.SatelliteNode.ID, nbr.SatelliteNode.ID, fragID)
													lock.Unlock()

													time.Sleep(time.Duration(delay * float64(time.Second)))

													lock.Lock()

													if !nbr.Fragments[fragID] {
														nbr.Fragments[fragID] = true

														fmt.Printf("[%.2fs] ✅ DFedSat inter-plane %d successfully received fragment %d from satellite %d\n",
															currentSimulationTimeDFedSat(), nbr.SatelliteNode.ID, fragID, node.SatelliteNode.ID)
													}
													remainingTime -= delay
													lock.Unlock()
												} else {
													lock.Unlock()
												}
											}

											for fragID := 0; fragID < globals.F(); fragID++ {
												lock.Lock()
												if nbr.Fragments[fragID] && !node.Fragments[fragID] {
													delay := alpha + float64(globals.FRAGMENT_SIZE_MB)/bw
													if delay > remainingTime {
														lock.Unlock()
														break
													}

													fmt.Printf("[%.2fs] 🛰️ DFedSat inter-plane %d→%d sending fragment %d\n",
														currentSimulationTimeDFedSat(), nbr.SatelliteNode.ID, node.SatelliteNode.ID, fragID)
													lock.Unlock()

													time.Sleep(time.Duration(delay * float64(time.Second)))

													lock.Lock()

													if !node.Fragments[fragID] {
														node.Fragments[fragID] = true

														fmt.Printf("[%.2fs] ✅ DFedSat inter-plane %d successfully received fragment %d from satellite %d\n",
															currentSimulationTimeDFedSat(), node.SatelliteNode.ID, fragID, nbr.SatelliteNode.ID)
													}
													remainingTime -= delay
													lock.Unlock()
												} else {
													lock.Unlock()
												}
											}
											break
										}
									}
								}
							}
						}()
					}

					lock.Lock()
					allDone := true
					for _, n := range dfedNodes {
						if len(n.Fragments) < globals.F() {
							allDone = false
							break
						}
					}
					lock.Unlock()

					if allDone {
						break
					}

					time.Sleep(sleepInterval)
				}
			}(i, dnode)
		}
	}

	wg.Wait()

	simEnd := currentSimulationTimeDFedSat()

	lock.Lock()
	completeCount := 0
	totalFrags := 0
	for i, node := range dfedNodes {
		if i > 0 && len(node.Fragments) >= globals.F() {
			completeCount++
		}
		totalFrags += len(node.Fragments)
	}
	lock.Unlock()

	fmt.Printf("\nDFedSat completed: %.2f seconds\n", simEnd)
	fmt.Printf("  Completed nodes: %d/%d (%.1f%%)\n", completeCount, len(dfedNodes)-1, float64(completeCount)*100/float64(len(dfedNodes)-1))
	fmt.Printf("  Total fragments in system: %d (theoretical: %d)\n", totalFrags, len(dfedNodes)*globals.F())
}
