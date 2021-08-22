package sixdegrees

import (
	"fmt"
	"log"
	"math"

	utils "github.com/Jonnymurillo288/GoUtils"
)

type Graph struct {
	Keys map[string]int
	Adj map[int][]Edge
}

func NewGraph() *Graph {
	return &Graph{
		Adj: make(map[int][]Edge),
		Keys: make(map[string]int),
	}
}

func (g *Graph) AddEdge(e Edge) {
	// get key from V
	v,ok := g.Keys[e.From()]; 
	if !ok { // if key doesnt exist
		v = len(g.Keys) // new key = end
		g.Keys[e.V.Name] = v 
	}
	// graph adj from key 
	g.Adj[v] = append(g.Adj[v],e)
}

type Edge struct {
	V,W *Artists
	Weight float64 // weight: abs(popularity target - popularity feat)
}

func NewEdge(target *Artists, from *Artists, to *Artists) Edge {
	return Edge {
		V: from,
		W: to,
		Weight: math.Abs(target.Popularity - to.Popularity),
	}
}

func (e Edge) To() string {
	if e.V == nil || e.W == nil{
		return "Error"
	}
	return e.W.Name
}

func (e Edge) From() string {
	if e.V == nil || e.W == nil{
		return "Error"
	}
	return e.V.Name
}


// ===================================================================== //
// 						   Dijkstra's Implementation                     //

type Dijkstra struct {
	DistTo []float64
	EdgeTo []Edge
	PQ utils.IndexMinPQ
}

func NewDijkstras(g *Graph, s *Artists) Dijkstra {
	d := &Dijkstra{
		EdgeTo: make([]Edge,len(g.Adj)+1),
		DistTo: make([]float64,len(g.Adj)+1),
		PQ: utils.NewIndexMinPQ(len(g.Adj)+1),
	}
	for v := 0; v < len(g.Adj); v++ {
		d.DistTo[v] = math.Inf(1)
	}
	key := g.Keys[s.Name]
	d.DistTo[key] = 0.0
	d.PQ.Insert(key,s,0.0)
	for !d.PQ.IsEmpty() {
		fmt.Println("Length of the pq is:",len(d.PQ.PQ))
		v,_ := d.PQ.DelMin()
		for _,e := range g.Adj[v] {
			d.relax(e,g.Keys)
		}
	}
	return (*d)
}


func (d *Dijkstra) relax(e Edge, keys map[string]int) {
	fmt.Println(e.From(),e.To())
	vv := e.From()
	if vv == "Error" {
		return
	}
	v := keys[vv]
	w := keys[e.To()]
	if v == w {
		d.PQ.Insert(w,e.W,d.DistTo[w])
	}
	if d.DistTo[w] >= d.DistTo[v] + e.Weight {
		d.DistTo[w] = d.DistTo[v] + e.Weight
		d.EdgeTo[w] = e
		if d.PQ.Contains(w) {
			log.Printf("PQ contains %v\n",w)
			// if w in the Priority queue and distTo[w] less than current distTo[w]
			// decrese the key
			d.PQ.DecreaseKey(w,e.W, d.DistTo[w])
		} else {
			// otherwise insert the key into the PQ
			fmt.Printf("\nInserting %v into PQ",w)
			d.PQ.Insert(w,e.W, d.DistTo[w])
		}
	}
}


