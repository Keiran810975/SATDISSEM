# Gossip

This project simulates data transmission from a **ground base station** to multiple **satellites**.  
It provides both **control group (Flooding)** and **experimental group (Gossip)** algorithms for comparison.

---

## 1. Experimental Setup

All initialization parameters are configured in **`globals.go`**:

- **`N`**: Total number of nodes = satellites + base stations  
  (In this project, the number of base stations is fixed to **1**, so `N = satellites + 1`).

- **`F`**: Number of data fragments.

- **`FRAGMENT_SIZE_MB`**: Size of each data fragment (MB).

- **`MUL`**: Time window compression factor (used to accelerate simulation).

- **`BASE_BW`**: Base bandwidth between satellites.

- **`EXTRA_BW`**: Bandwidth fluctuation range (randomized variation).

---

## 2. Importing the Satellite Contact Plan

You must provide a **JSON file** containing the satellite contact time windows.  
This file defines when two nodes (base station or satellites) are able to communicate.

### Example format:

```json
{
  "0-1": [
    [1737.49, 1854.33],
    [42979.52, 43185.84]
  ],

  "0-2": [
    [43691.06, 43888.0]
  ],

 ......


  "30-32": [],

  "31-32": [
    [0, 86340.0]
  ]
}
```

- Keys such as `"30-32"` denote the connection between **satellite 30** and **satellite 32**.

- **`0`** always refers to the **base station**.

- Each pair `[start, end]` specifies a **communication time interval** in **seconds**.

---

## 3. Running the Simulation

1. Place your contact plan JSON file in the project root directory.

2. In **`main.go`**, set the following variable:

```go
topologyFile = "your_topology.json"
```

3. Select the algorithm by commenting/uncommenting one of the following lines:

```go
// Run control group (Flooding)
algorithms.RunSimulationFlooding(nodes)

// Run experimental group (Gossip)
algorithms.RunSimulationExperiment(nodes)
```
