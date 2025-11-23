// ---------------------------------------------------------
// GLOBALS
// ---------------------------------------------------------
let graph = new graphology.Graph();
let renderer = null;
let forceAtlas = null;

const ARTIST_COLOR = "#2ecc71";
const START_COLOR = "#2779bd";
const TARGET_COLOR = "#c53030";
const EDGE_COLOR = "#e74c3c"; // Track edges



// ---------------------------------------------------------
// Add ARTIST node only (no track nodes)
// ---------------------------------------------------------
function ensureArtistNode(name, start, target) {
  if (!graph.hasNode(name)) {
    let color = ARTIST_COLOR;
    let size = 8;

    if (name === start) {
      color = START_COLOR;
      size = 12;
    } else if (name === target) {
      color = TARGET_COLOR;
      size = 12;
    }

    graph.addNode(name, {
      label: name,
      type: "artist",
      color,
      size,
      x: Math.random(),
      y: Math.random(),
    });
  }
}



// ---------------------------------------------------------
// Add one hop (edge = track)
// ---------------------------------------------------------
function addStep(step, start, target) {
  const from = step.from;
  const to = step.to;
  const track = step.track;

  if (!from || !to || !track) return;

  // Artists only
  ensureArtistNode(from, start, target);
  ensureArtistNode(to, start, target);

  const edgeId = `${from}->${to}-${track}`;
  if (!graph.hasEdge(edgeId)) {
    graph.addEdge(edgeId, from, to, {
      label: track,
      size: 2,
      color: EDGE_COLOR,
    });
  }
}



// ---------------------------------------------------------
// Add path
// ---------------------------------------------------------
function addPathToGraph(path, start, target) {
  path.forEach(s => addStep(s, start, target));
}



// ---------------------------------------------------------
// Initialize Sigma
// ---------------------------------------------------------
function initGraph() {
  graph.clear();

  const container = document.getElementById("graph-container");
  container.innerHTML = "";

  renderer = new Sigma(graph, container, {
    renderLabels: true,
    labelDensity: 0.5,
  });

  enableArtistExpansion();
}



// ---------------------------------------------------------
// Click artist -> expand
// ---------------------------------------------------------
function enableArtistExpansion() {
  renderer.on("clickNode", async ({ node }) => {
    const attrs = graph.getNodeAttributes(node);
    if (attrs.type !== "artist") return;

    if (attrs.expanded) return;
    graph.setNodeAttribute(node, "expanded", true);

    try {
      const res = await fetch("/expandNode", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({ artist: node }),
      });

      if (!res.ok) return;

      const steps = await res.json();
      steps.forEach(s => addStep(s, null, null));

      restartForceAtlas();

    } catch (err) {
      console.error("Expand error:", err);
    }
  });
}



// ---------------------------------------------------------
// ForceAtlas2 Layout
// ---------------------------------------------------------
function restartForceAtlas() {
  if (forceAtlas) forceAtlas.kill();

  forceAtlas = graphologyLibraryLayoutForceAtlas2(graph, {
    settings: {
      gravity: 0.02,
      scalingRatio: 10,
      slowDown: 2,
    },
  });

  forceAtlas.start();
  setTimeout(() => forceAtlas.stop(), 1500);
}



// ---------------------------------------------------------
// Search Handler
// ---------------------------------------------------------
async function runSearch() {
  const start = document.getElementById("startArtist").value.trim();
  const target = document.getElementById("targetArtist").value.trim();
  const depth = Number(document.getElementById("depth").value || -1);

  const results = document.getElementById("results");
  const spinner = document.getElementById("spinner");

  spinner.style.display = "inline-block";
  results.innerHTML = "";

  try {
    const res = await fetch("/search", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify({ start, target, depth }),
    });

    spinner.style.display = "none";
    const data = await res.json();

    results.innerHTML = `
      <p><strong>Start:</strong> ${data.start}<br>
         <strong>Target:</strong> ${data.target}<br>
         <strong>Hops:</strong> ${data.hops}</p>`;

    initGraph();
    addPathToGraph(data.path, start, target);
    restartForceAtlas();

  } catch (err) {
    spinner.style.display = "none";
    results.innerHTML = `<p class="error">Search failed.</p>`;
    console.error(err);
  }
}



// ---------------------------------------------------------
// Form Bind
// ---------------------------------------------------------
document.getElementById("searchForm").addEventListener("submit", (e) => {
  e.preventDefault();
  runSearch();
});
