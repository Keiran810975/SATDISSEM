package algorithms

import (
	"fmt"
	"sate/globals"
	"sate/model"
	"sort"
	"time"
)

func transmitFromGroundOnly(ground *model.SatelliteNode, satellite *model.SatelliteNode) {
	now := currentSimulationTime()

	if !ground.IsConnected(satellite.ID, now) {
		return
	}

	alpha := globals.TransmissionDelays[ground.ID][satellite.ID]
	bandwidth := ground.GetBandwidth(satellite.ID)
	if bandwidth <= 0 {
		return
	}

	delay := alpha + globals.FRAGMENT_SIZE_MB/bandwidth
	intervals := ground.DynamicNeighbors[satellite.ID]

	for _, iv := range intervals {
		if iv.Start <= now && now < iv.End {
			remainingTime := iv.End - now

			lock.Lock()
			fragList := []int{}
			for f := range ground.Fragments {
				if _, ok := satellite.Fragments[f]; !ok {
					fragList = append(fragList, f)
				}
			}
			lock.Unlock()

			sort.Ints(fragList)

			for _, frag := range fragList {
				lock.Lock()
				if _, ok := satellite.Fragments[frag]; ok {
					lock.Unlock()
					continue
				}
				fmt.Printf("[%.2fs] 🛰️ Ground station sending fragment %d to satellite %d\n", currentSimulationTime(), satellite.ID, frag)
				lock.Unlock()

				time.Sleep(time.Duration(delay * float64(time.Second)))

				lock.Lock()
				satellite.Fragments[frag] = struct{}{}
				fmt.Printf("[%.2fs] ✅ Satellite %d successfully received fragment %d from ground station\n", currentSimulationTime(), satellite.ID, frag)
				lock.Unlock()

				now += delay
				remainingTime -= delay

				if delay > remainingTime {
					break
				}
			}
			break
		}
	}
}

func RunSimulationGroundOnly(nodes []*model.SatelliteNode) {
	for i := 0; i < globals.F(); i++ {
		nodes[0].Fragments[i] = struct{}{}
	}

	for {
		allDone := true
		lock.Lock()
		for i := 1; i < len(nodes); i++ {
			if len(nodes[i].Fragments) < globals.F() {
				allDone = false
				break
			}
		}
		lock.Unlock()
		if allDone {
			break
		}

		for i := 1; i < len(nodes); i++ {
			transmitFromGroundOnly(nodes[0], nodes[i])
		}

		time.Sleep(10 * time.Millisecond)
	}

	fmt.Printf("\n✅ All satellites completed ground transmission, total time: %.2f seconds\n", currentSimulationTime())
}
