(() => {
  const dagEl = document.getElementById("dag");
  const panelEl = document.getElementById("detail-panel");
  const branchSelectEl = document.getElementById("branch-select");
  const zoomLevelEl = document.getElementById("zoom-level");
  const zoomInEl = document.getElementById("zoom-in");
  const zoomOutEl = document.getElementById("zoom-out");
  const zoomResetEl = document.getElementById("zoom-reset");
  const zoomFitEl = document.getElementById("zoom-fit");
  const presetWinnerEl = document.getElementById("preset-winner");
  const presetActiveEl = document.getElementById("preset-active");
  const presetAllEl = document.getElementById("preset-all");
  const lineageToggleEl = document.getElementById("lineage-toggle");

  const winnerHeroEl = document.getElementById("winner-hero");
  const winnerKpisEl = document.getElementById("winner-kpis");
  const winnerReasonEl = document.getElementById("winner-reason");
  const quickCompareEl = document.getElementById("quick-compare");
  const compareASelectEl = document.getElementById("compare-a-select");
  const compareBSelectEl = document.getElementById("compare-b-select");
  const compareRunEl = document.getElementById("compare-run");

  const tabWinnerEl = document.getElementById("tab-winner");
  const tabLineageEl = document.getElementById("tab-lineage");
  const experienceEl = document.getElementById("experience");
  const winnerPanelEl = document.getElementById("winner-panel");
  const lineagePanelEl = document.getElementById("lineage-panel");

  if (!dagEl || !panelEl || !branchSelectEl) {
    return;
  }

  const SVG_NS = "http://www.w3.org/2000/svg";
  const NODE_WIDTH = 224;
  const NODE_HEIGHT = 122;

  let allCells = [];
  let allBranches = [];
  let uiSummary = null;
  let compareStart = null;
  let didRunDefaultCompare = false;

  const manualCompare = {
    cellA: "",
    cellB: "",
  };

  const graphState = {
    byID: new Map(),
    nodes: new Map(),
    edges: [],
    selectedIDs: new Set(),
    edgeElsByID: new Map(),
    nodeElsByID: new Map(),
    lineageIDs: new Set(),
    winnerID: "",
    showFullLineage: true,
    zoom: { x: 0, y: 0, k: 1 },
    drag: null,
    pan: null,
    sceneEl: null,
    edgeLayerEl: null,
    nodeLayerEl: null,
  };

  async function init() {
    bindViewportControls();
    bindCanvasGestures();
    bindCompareControls();
    bindTabs();
    setPanelTab("lineage");
    bindLineageToggle();

    await loadData();
    renderBranchFilter();
    renderWinnerCockpit();
    renderCompareSelectors();
    renderGraph();
    await applyInitialWinnerState();
  }

  function bindViewportControls() {
    zoomInEl?.addEventListener("click", () => zoomBy(1.2));
    zoomOutEl?.addEventListener("click", () => zoomBy(0.84));
    zoomResetEl?.addEventListener("click", () => resetZoom());
    zoomFitEl?.addEventListener("click", () => fitToGraph(true));
    presetWinnerEl?.addEventListener("click", () => focusWinner(true));
    presetActiveEl?.addEventListener("click", () => focusActiveBranch(true));
    presetAllEl?.addEventListener("click", () => fitToGraph(true));
  }

  function bindCanvasGestures() {
    dagEl.addEventListener(
      "wheel",
      (event) => {
        if (!graphState.sceneEl) {
          return;
        }
        event.preventDefault();

        const factor = event.deltaY < 0 ? 1.08 : 0.92;
        const p = clientToSvg(event.clientX, event.clientY);
        const before = graphToScreen(p.x, p.y);

        graphState.zoom.k = clamp(graphState.zoom.k * factor, 0.32, 2.8);
        const after = graphToScreen(p.x, p.y);

        graphState.zoom.x += before.x - after.x;
        graphState.zoom.y += before.y - after.y;
        applySceneTransform();
      },
      { passive: false },
    );

    dagEl.addEventListener("pointerdown", (event) => {
      const node = event.target.closest(".node");
      if (node || event.button !== 0) {
        return;
      }
      event.preventDefault();
      graphState.pan = {
        pointerID: event.pointerId,
        startClientX: event.clientX,
        startClientY: event.clientY,
        startZoomX: graphState.zoom.x,
        startZoomY: graphState.zoom.y,
      };
      dagEl.setPointerCapture(event.pointerId);
    });

    dagEl.addEventListener("pointermove", (event) => {
      const pan = graphState.pan;
      if (!pan || pan.pointerID !== event.pointerId) {
        return;
      }

      graphState.zoom.x = pan.startZoomX + (event.clientX - pan.startClientX);
      graphState.zoom.y = pan.startZoomY + (event.clientY - pan.startClientY);
      applySceneTransform();
    });

    const endPan = (event) => {
      const pan = graphState.pan;
      if (!pan || pan.pointerID !== event.pointerId) {
        return;
      }
      graphState.pan = null;
      if (dagEl.hasPointerCapture(event.pointerId)) {
        dagEl.releasePointerCapture(event.pointerId);
      }
    };

    dagEl.addEventListener("pointerup", endPan);
    dagEl.addEventListener("pointercancel", endPan);
  }

  function bindCompareControls() {
    compareASelectEl?.addEventListener("change", () => {
      manualCompare.cellA = compareASelectEl.value;
    });

    compareBSelectEl?.addEventListener("change", () => {
      manualCompare.cellB = compareBSelectEl.value;
    });

    compareRunEl?.addEventListener("click", async () => {
      if (!manualCompare.cellA || !manualCompare.cellB) {
        panelEl.innerHTML = '<div class="placeholder">Choose both comparison cells first.</div>';
        return;
      }
      await renderCompare(manualCompare.cellA, manualCompare.cellB);
    });

    quickCompareEl?.addEventListener("click", async () => {
      const winnerID = (uiSummary?.winner_cell_id || "").trim();
      const baselineID = (uiSummary?.baseline_cell_id || "").trim();
      if (!winnerID || !baselineID || winnerID === baselineID) {
        panelEl.innerHTML = '<div class="placeholder">Winner vs baseline is unavailable for this history.</div>';
        return;
      }

      setCompareSelections(baselineID, winnerID);
      await renderCompare(baselineID, winnerID);
      setSelected([baselineID, winnerID]);
    });
  }

  function bindTabs() {
    tabWinnerEl?.addEventListener("click", () => setPanelTab("winner"));
    tabLineageEl?.addEventListener("click", () => setPanelTab("lineage"));
  }

  function bindLineageToggle() {
    lineageToggleEl?.addEventListener("click", () => {
      graphState.showFullLineage = !graphState.showFullLineage;
      applyLineageClasses();
      renderLineageToggle();
    });
  }

  function setPanelTab(panel) {
    if (!experienceEl) {
      return;
    }
    const showWinner = panel !== "lineage";
    experienceEl.classList.toggle("show-winner", showWinner);
    experienceEl.classList.toggle("show-lineage", !showWinner);

    setTabState(tabWinnerEl, showWinner);
    setTabState(tabLineageEl, !showWinner);
    winnerPanelEl?.setAttribute("aria-hidden", String(!showWinner));
    lineagePanelEl?.setAttribute("aria-hidden", String(showWinner));
  }

  function setTabState(el, active) {
    if (!el) {
      return;
    }
    el.classList.toggle("active", active);
    el.setAttribute("aria-selected", String(active));
    el.tabIndex = active ? 0 : -1;
  }

  async function loadData() {
    const [cellsResp, branchesResp, summaryResp] = await Promise.all([
      fetch("/api/cells"),
      fetch("/api/branches"),
      fetch("/api/ui/summary"),
    ]);

    if (!cellsResp.ok) {
      throw new Error(`/api/cells failed: ${cellsResp.status}`);
    }
    if (!branchesResp.ok) {
      throw new Error(`/api/branches failed: ${branchesResp.status}`);
    }

    allCells = await cellsResp.json();
    allBranches = await branchesResp.json();

    if (summaryResp.ok) {
      uiSummary = await summaryResp.json();
    } else {
      uiSummary = buildFallbackSummary();
    }

    if (!uiSummary) {
      uiSummary = buildFallbackSummary();
    }
  }

  function buildFallbackSummary() {
    const active = allBranches.find((b) => b.active);
    const winner = pickWinnerCellClient(allCells);
    const baseline = allCells[0];
    return {
      total_cells: allCells.length,
      total_branches: allBranches.length,
      active_branch: active?.name || "main",
      winner_cell_id: winner?.id || "",
      baseline_cell_id: baseline?.id || "",
      pass_rate: calculatePassRateClient(allCells),
      fork_points: countForkPointsClient(allCells),
    };
  }

  function renderBranchFilter() {
    branchSelectEl.innerHTML = "";

    const allOpt = document.createElement("option");
    allOpt.value = "";
    allOpt.textContent = "All branches";
    allOpt.selected = true;
    branchSelectEl.appendChild(allOpt);

    for (const branch of allBranches) {
      const opt = document.createElement("option");
      opt.value = branch.name;
      opt.textContent = branch.active ? `${branch.name} (active)` : branch.name;
      branchSelectEl.appendChild(opt);
    }

    branchSelectEl.addEventListener("change", () => {
      compareStart = null;
      renderGraph();
    });
  }

  function renderWinnerCockpit() {
    const winner = cellByID(uiSummary?.winner_cell_id);
    const baseline = cellByID(uiSummary?.baseline_cell_id);

    if (!winner) {
      winnerHeroEl.classList.remove("loading");
      winnerHeroEl.innerHTML = "<strong>No winner yet</strong>Take at least one snapshot to unlock winner insights.";
      winnerKpisEl.innerHTML = "";
      winnerReasonEl.innerHTML = '<div class="reason-card"><div class="reason-title">Why it wins</div><div class="placeholder">No experiment data yet.</div></div>';
      quickCompareEl.disabled = true;
      return;
    }

    winnerHeroEl.classList.remove("loading");
    winnerHeroEl.innerHTML = `
      <strong>Best attempt: ${escapeHtml(winner.id)}</strong>
      ${escapeHtml(winner.message || "(no message)")}<br>
      Winner confidence: ${formatPercent(uiSummary?.pass_rate || 0)} pass reliability from evaluated runs.
    `;

    winnerKpisEl.innerHTML = `
      ${kpiCard("Total Cells", String(uiSummary?.total_cells || 0), "Recorded experiments")}
      ${kpiCard("Active Branch", escapeHtml(uiSummary?.active_branch || "main"), "Current default target")}
      ${kpiCard("Pass Rate", formatPercent(uiSummary?.pass_rate || 0), "From evaluated cells")}
      ${kpiCard("Fork Points", String(uiSummary?.fork_points || 0), "Decision moments")}
    `;

    const winnerReasons = buildWinnerReasonLines(winner, baseline);
    const tradeoffs = buildTradeoffLines(winner, baseline);
    winnerReasonEl.innerHTML = `
      <article class="reason-card">
        <div class="reason-title">Why It Wins</div>
        <ul class="reason-list">${winnerReasons.map((line) => `<li>${escapeHtml(line)}</li>`).join("")}</ul>
      </article>
      <article class="reason-card">
        <div class="reason-title">Tradeoffs</div>
        <ul class="reason-list">${tradeoffs.map((line) => `<li>${escapeHtml(line)}</li>`).join("")}</ul>
      </article>
    `;

    quickCompareEl.disabled = !baseline || baseline.id === winner.id;
  }

  function kpiCard(label, value, note) {
    return `
      <article class="kpi-card">
        <div class="kpi-label">${escapeHtml(label)}</div>
        <div class="kpi-value">${escapeHtml(value)}</div>
        <div class="kpi-note">${escapeHtml(note)}</div>
      </article>
    `;
  }

  function buildWinnerReasonLines(winner, baseline) {
    const lines = [];

    if (winner.tests_failed != null) {
      lines.push(`Tests failed: ${winner.tests_failed} (lower is better).`);
    }
    const lintType = ptrInt(winner.lint_errors) + ptrInt(winner.type_errors);
    lines.push(`Lint + type errors: ${lintType}.`);

    if (winner.tests_passed != null) {
      lines.push(`Tests passed: ${winner.tests_passed}.`);
    }

    if (baseline) {
      lines.push(`Compared against baseline ${baseline.id} to keep improvements measurable.`);
    }

    if (!lines.length) {
      lines.push("No evaluation data is available; latest attempt is selected as best by default.");
    }

    return lines;
  }

  function buildTradeoffLines(winner, baseline) {
    const lines = [];

    lines.push(`LOC delta: ${signed(winner.loc_delta)} (total ${winner.total_loc}).`);
    lines.push(`File movement: +${winner.files_added} ~${winner.files_modified} -${winner.files_removed}.`);

    if (baseline) {
      lines.push(`Baseline vs winner path: ${baseline.id} -> ${winner.id}.`);
    }

    if (winner.tests_failed == null && winner.lint_errors == null && winner.type_errors == null) {
      lines.push("Tradeoff: selection confidence is lower because eval metrics are missing.");
    }

    return lines;
  }

  function renderCompareSelectors() {
    const options = allCells
      .slice()
      .sort((a, b) => b.sequence - a.sequence)
      .map((cell) => ({
        value: cell.id,
        label: `${cell.id} - ${truncate(cell.message || "(no message)", 32)}`,
      }));

    if (compareASelectEl) {
      compareASelectEl.innerHTML = options
        .map((opt) => `<option value="${escapeHtml(opt.value)}">${escapeHtml(opt.label)}</option>`)
        .join("");
    }
    if (compareBSelectEl) {
      compareBSelectEl.innerHTML = options
        .map((opt) => `<option value="${escapeHtml(opt.value)}">${escapeHtml(opt.label)}</option>`)
        .join("");
    }

    const fallbackLatest = allCells.at(-1)?.id || "";
    const defaultA = uiSummary?.baseline_cell_id || fallbackLatest;
    const defaultB = uiSummary?.winner_cell_id || fallbackLatest;
    setCompareSelections(defaultA, defaultB);
  }

  function setCompareSelections(cellA, cellB) {
    manualCompare.cellA = cellA || "";
    manualCompare.cellB = cellB || "";

    if (compareASelectEl && manualCompare.cellA) {
      compareASelectEl.value = manualCompare.cellA;
    }
    if (compareBSelectEl && manualCompare.cellB) {
      compareBSelectEl.value = manualCompare.cellB;
    }
  }

  function selectedBranch() {
    return branchSelectEl.value.trim();
  }

  function filteredCells() {
    const branch = selectedBranch();
    if (!branch) {
      return [...allCells];
    }
    return allCells.filter((cell) => cell.branch === branch);
  }

  function renderGraph() {
    const cells = filteredCells().sort((a, b) => a.sequence - b.sequence);

    dagEl.innerHTML = "";

    graphState.byID = new Map(cells.map((c) => [c.id, c]));
    graphState.nodes = new Map();
    graphState.edges = [];
    graphState.selectedIDs = new Set();
    graphState.edgeElsByID = new Map();
    graphState.nodeElsByID = new Map();
    graphState.lineageIDs = new Set();
    graphState.sceneEl = null;
    graphState.edgeLayerEl = null;
    graphState.nodeLayerEl = null;

    if (!cells.length) {
      renderEmpty();
      updateZoomLabel(1);
      return;
    }

    initializeLayout(cells);
    buildScene();
    renderEdges();
    renderNodes();

    graphState.winnerID = uiSummary?.winner_cell_id || "";
    computeLineageIDs(graphState.winnerID);
    applyLineageClasses();
    applySelectionClasses();
    fitToGraph(false);
  }

  function initializeLayout(cells) {
    const byID = graphState.byID;

    const childrenByParent = new Map();
    for (const cell of cells) {
      if (!cell.parent_id || !byID.has(cell.parent_id)) {
        continue;
      }
      if (!childrenByParent.has(cell.parent_id)) {
        childrenByParent.set(cell.parent_id, []);
      }
      childrenByParent.get(cell.parent_id).push(cell.id);
    }

    for (const children of childrenByParent.values()) {
      children.sort((a, b) => (byID.get(a)?.sequence || 0) - (byID.get(b)?.sequence || 0));
    }

    const laneByID = new Map();
    const roots = cells.filter((cell) => !cell.parent_id || !byID.has(cell.parent_id));
    roots.sort((a, b) => a.sequence - b.sequence);

    let nextLane = 0;
    const assignLane = (id, lane) => {
      if (laneByID.has(id)) {
        return;
      }
      laneByID.set(id, lane);

      const children = childrenByParent.get(id) || [];
      children.forEach((childID, idx) => {
        if (idx === 0) {
          assignLane(childID, lane);
          return;
        }
        const branchLane = nextLane;
        nextLane += 1;
        assignLane(childID, branchLane);
      });
    };

    for (const root of roots) {
      const rootLane = nextLane;
      nextLane += 1;
      assignLane(root.id, rootLane);
    }

    for (const cell of cells) {
      if (!laneByID.has(cell.id)) {
        laneByID.set(cell.id, nextLane);
        nextLane += 1;
      }
    }

    const marginX = 76;
    const marginY = 68;
    const xStep = 252;
    const yStep = 168;

    for (let i = 0; i < cells.length; i += 1) {
      const cell = cells[i];
      graphState.nodes.set(cell.id, {
        id: cell.id,
        cell,
        width: NODE_WIDTH,
        height: NODE_HEIGHT,
        x: marginX + i * xStep,
        y: marginY + (laneByID.get(cell.id) || 0) * yStep,
      });
    }

    for (const cell of cells) {
      if (!cell.parent_id || !byID.has(cell.parent_id)) {
        continue;
      }
      graphState.edges.push({
        id: `${cell.parent_id}=>${cell.id}`,
        source: cell.parent_id,
        target: cell.id,
      });
    }

    const width = Math.max(1200, marginX * 2 + cells.length * xStep + NODE_WIDTH);
    const height = Math.max(760, marginY * 2 + nextLane * yStep + NODE_HEIGHT);
    dagEl.setAttribute("viewBox", `0 0 ${width} ${height}`);
  }

  function buildScene() {
    const defs = document.createElementNS(SVG_NS, "defs");
    const marker = document.createElementNS(SVG_NS, "marker");
    marker.setAttribute("id", "edge-arrow");
    marker.setAttribute("viewBox", "0 -5 10 10");
    marker.setAttribute("refX", "9");
    marker.setAttribute("refY", "0");
    marker.setAttribute("markerWidth", "7");
    marker.setAttribute("markerHeight", "7");
    marker.setAttribute("orient", "auto");

    const markerPath = document.createElementNS(SVG_NS, "path");
    markerPath.setAttribute("d", "M0,-5L10,0L0,5");
    markerPath.setAttribute("class", "edge-arrow");
    marker.appendChild(markerPath);
    defs.appendChild(marker);
    dagEl.appendChild(defs);

    const scene = document.createElementNS(SVG_NS, "g");
    scene.setAttribute("class", "dag-scene");

    const edgeLayer = document.createElementNS(SVG_NS, "g");
    edgeLayer.setAttribute("class", "edge-layer");

    const nodeLayer = document.createElementNS(SVG_NS, "g");
    nodeLayer.setAttribute("class", "node-layer");

    scene.appendChild(edgeLayer);
    scene.appendChild(nodeLayer);
    dagEl.appendChild(scene);

    graphState.sceneEl = scene;
    graphState.edgeLayerEl = edgeLayer;
    graphState.nodeLayerEl = nodeLayer;
  }

  function renderEdges() {
    for (const edge of graphState.edges) {
      const path = document.createElementNS(SVG_NS, "path");
      path.setAttribute("class", "edge");
      path.setAttribute("marker-end", "url(#edge-arrow)");
      path.setAttribute("d", edgePath(edge));
      graphState.edgeLayerEl.appendChild(path);
      graphState.edgeElsByID.set(edge.id, path);
    }
  }

  function renderNodes() {
    const nodeEntries = Array.from(graphState.nodes.values());
    for (const node of nodeEntries) {
      const g = document.createElementNS(SVG_NS, "g");
      g.setAttribute("class", `node ${statusClass(node.cell)}`);
      g.setAttribute("data-id", node.id);
      g.setAttribute("transform", `translate(${node.x}, ${node.y})`);
      g.setAttribute("tabindex", "0");
      g.setAttribute("role", "button");
      g.setAttribute("aria-label", `${node.id} ${node.cell.message || ""}`);

      g.addEventListener("pointerdown", (event) => onNodePointerDown(event, node.id));
      g.addEventListener("keydown", (event) => {
        if (event.key !== "Enter" && event.key !== " ") {
          return;
        }
        event.preventDefault();
        void activateNode(node.id, event.shiftKey);
      });

      const shell = document.createElementNS(SVG_NS, "rect");
      shell.setAttribute("class", "node-shell");
      shell.setAttribute("width", String(NODE_WIDTH));
      shell.setAttribute("height", String(NODE_HEIGHT));
      shell.setAttribute("rx", "10");
      shell.setAttribute("ry", "10");
      g.appendChild(shell);

      const dot = document.createElementNS(SVG_NS, "circle");
      dot.setAttribute("class", "status-dot");
      dot.setAttribute("cx", "14");
      dot.setAttribute("cy", "16");
      dot.setAttribute("r", "4.5");
      g.appendChild(dot);

      const pillBg = document.createElementNS(SVG_NS, "rect");
      pillBg.setAttribute("class", "status-pill-bg");
      pillBg.setAttribute("x", String(NODE_WIDTH - 70));
      pillBg.setAttribute("y", "9");
      pillBg.setAttribute("width", "58");
      pillBg.setAttribute("height", "19");
      pillBg.setAttribute("rx", "9");
      pillBg.setAttribute("ry", "9");
      g.appendChild(pillBg);

      g.appendChild(svgText(NODE_WIDTH - 41, 23, statusLabel(node.cell), "status-pill", "middle"));
      g.appendChild(svgText(26, 20, node.id, "id"));
      const messageLines = wrapTextLines(node.cell.message || "(no message)", 28, 3);
      appendWrappedSvgText(g, 14, 45, messageLines, "message", 14);
      const wrappedOffset = (messageLines.length - 1) * 14;
      g.appendChild(svgText(14, 65 + wrappedOffset, `parent: ${node.cell.parent_id || "root"}`, "meta"));
      g.appendChild(svgText(14, 84 + wrappedOffset, `loc ${signed(node.cell.loc_delta)} (total ${node.cell.total_loc})`, "meta"));

      graphState.nodeLayerEl.appendChild(g);
      graphState.nodeElsByID.set(node.id, g);
    }
  }

  function renderEmpty() {
    const text = document.createElementNS(SVG_NS, "text");
    text.setAttribute("x", "32");
    text.setAttribute("y", "42");
    text.setAttribute("class", "meta");
    text.textContent = "No cells for the selected branch.";
    dagEl.appendChild(text);
  }

  function onNodePointerDown(event, nodeID) {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();

    const point = clientToSvg(event.clientX, event.clientY);
    const node = graphState.nodes.get(nodeID);
    if (!node) {
      return;
    }

    graphState.drag = {
      pointerID: event.pointerId,
      nodeID,
      startPointerX: point.x,
      startPointerY: point.y,
      startNodeX: node.x,
      startNodeY: node.y,
      moved: false,
      captureEl: graphState.nodeElsByID.get(nodeID) || null,
    };

    const nodeEl = graphState.drag.captureEl;
    nodeEl?.classList.add("dragging");
    nodeEl?.parentNode?.appendChild(nodeEl);
    nodeEl?.setPointerCapture(event.pointerId);
  }

  dagEl.addEventListener("pointermove", (event) => {
    const drag = graphState.drag;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }

    const current = clientToSvg(event.clientX, event.clientY);
    const dx = current.x - drag.startPointerX;
    const dy = current.y - drag.startPointerY;

    if (Math.abs(dx) > 2 || Math.abs(dy) > 2) {
      drag.moved = true;
    }

    const node = graphState.nodes.get(drag.nodeID);
    if (!node) {
      return;
    }

    node.x = drag.startNodeX + dx;
    node.y = drag.startNodeY + dy;

    updateNodePosition(node.id);
    updateEdgesForNode(node.id);
  });

  const endDrag = (event) => {
    const drag = graphState.drag;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }

    const nodeEl = graphState.nodeElsByID.get(drag.nodeID);
    nodeEl?.classList.remove("dragging");

    if (!drag.moved) {
      void activateNode(drag.nodeID, event.shiftKey);
    }

    graphState.drag = null;
    if (nodeEl?.hasPointerCapture(event.pointerId)) {
      nodeEl.releasePointerCapture(event.pointerId);
    }
  };

  dagEl.addEventListener("pointerup", endDrag);
  dagEl.addEventListener("pointercancel", endDrag);

  function updateNodePosition(nodeID) {
    const node = graphState.nodes.get(nodeID);
    const g = graphState.nodeElsByID.get(nodeID);
    if (!node || !g) {
      return;
    }
    g.setAttribute("transform", `translate(${node.x}, ${node.y})`);
  }

  function updateEdgesForNode(nodeID) {
    for (const edge of graphState.edges) {
      if (edge.source !== nodeID && edge.target !== nodeID) {
        continue;
      }
      const pathEl = graphState.edgeElsByID.get(edge.id);
      if (!pathEl) {
        continue;
      }
      pathEl.setAttribute("d", edgePath(edge));
    }
  }

  function edgePath(edge) {
    const source = graphState.nodes.get(edge.source);
    const target = graphState.nodes.get(edge.target);
    if (!source || !target) {
      return "";
    }

    const sx = source.x + source.width;
    const sy = source.y + source.height / 2;
    const tx = target.x;
    const ty = target.y + target.height / 2;

    const delta = Math.max(42, Math.abs(tx - sx) * 0.45);
    const c1x = sx + delta;
    const c1y = sy;
    const c2x = tx - delta;
    const c2y = ty;

    return `M ${sx} ${sy} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${tx} ${ty}`;
  }

  function focusWinner(animate) {
    const lineageIDs = Array.from(graphState.lineageIDs);
    if (lineageIDs.length > 0) {
      fitToNodeIDs(lineageIDs, animate);
      return;
    }

    const winnerID = graphState.winnerID || uiSummary?.winner_cell_id;
    if (winnerID && graphState.nodes.has(winnerID)) {
      fitToNodeIDs([winnerID], animate);
      return;
    }

    fitToGraph(animate);
  }

  function focusActiveBranch(animate) {
    const active = selectedBranch() || uiSummary?.active_branch || "";
    const ids = [];
    for (const [id, node] of graphState.nodes.entries()) {
      if (node.cell.branch === active) {
        ids.push(id);
      }
    }
    if (!ids.length) {
      fitToGraph(animate);
      return;
    }
    fitToNodeIDs(ids, animate);
  }

  function fitToGraph(animate) {
    fitToNodeIDs(Array.from(graphState.nodes.keys()), animate);
  }

  function fitToNodeIDs(ids, animate) {
    const bounds = computeContentBounds(ids);
    if (!bounds) {
      return;
    }

    const viewportWidth = dagEl.clientWidth || 1200;
    const viewportHeight = dagEl.clientHeight || 720;
    const pad = 74;

    const scale = clamp(
      Math.min((viewportWidth - pad) / bounds.width, (viewportHeight - pad) / bounds.height, 1.5),
      0.32,
      2.8,
    );

    const x = (viewportWidth - bounds.width * scale) / 2 - bounds.minX * scale;
    const y = (viewportHeight - bounds.height * scale) / 2 - bounds.minY * scale;

    if (!animate) {
      graphState.zoom = { x, y, k: scale };
      applySceneTransform();
      return;
    }

    animateZoomTo({ x, y, k: scale }, 220);
  }

  function animateZoomTo(target, durationMs) {
    const start = { ...graphState.zoom };
    const startTime = performance.now();

    const tick = (now) => {
      const t = Math.min(1, (now - startTime) / durationMs);
      const eased = 1 - Math.pow(1 - t, 3);

      graphState.zoom.x = start.x + (target.x - start.x) * eased;
      graphState.zoom.y = start.y + (target.y - start.y) * eased;
      graphState.zoom.k = start.k + (target.k - start.k) * eased;
      applySceneTransform();

      if (t < 1) {
        requestAnimationFrame(tick);
      }
    };

    requestAnimationFrame(tick);
  }

  function computeContentBounds(nodeIDs) {
    if (!nodeIDs.length) {
      return null;
    }

    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;

    for (const nodeID of nodeIDs) {
      const node = graphState.nodes.get(nodeID);
      if (!node) {
        continue;
      }
      minX = Math.min(minX, node.x);
      minY = Math.min(minY, node.y);
      maxX = Math.max(maxX, node.x + node.width);
      maxY = Math.max(maxY, node.y + node.height);
    }

    if (!Number.isFinite(minX) || !Number.isFinite(minY)) {
      return null;
    }

    return {
      minX,
      minY,
      maxX,
      maxY,
      width: Math.max(120, maxX - minX),
      height: Math.max(120, maxY - minY),
    };
  }

  function applySceneTransform() {
    if (!graphState.sceneEl) {
      updateZoomLabel(graphState.zoom.k);
      return;
    }
    const { x, y, k } = graphState.zoom;
    graphState.sceneEl.setAttribute("transform", `translate(${x}, ${y}) scale(${k})`);
    updateZoomLabel(k);
  }

  function graphToScreen(gx, gy) {
    const { x, y, k } = graphState.zoom;
    return {
      x: gx * k + x,
      y: gy * k + y,
    };
  }

  function screenToGraph(sx, sy) {
    const { x, y, k } = graphState.zoom;
    return {
      x: (sx - x) / k,
      y: (sy - y) / k,
    };
  }

  function clientToSvg(clientX, clientY) {
    const rect = dagEl.getBoundingClientRect();
    const localX = clientX - rect.left;
    const localY = clientY - rect.top;
    return screenToGraph(localX, localY);
  }

  function clamp(value, low, high) {
    return Math.max(low, Math.min(high, value));
  }

  function zoomBy(factor) {
    graphState.zoom.k = clamp(graphState.zoom.k * factor, 0.32, 2.8);
    applySceneTransform();
  }

  function resetZoom() {
    graphState.zoom = { x: 0, y: 0, k: 1 };
    applySceneTransform();
  }

  function updateZoomLabel(scale) {
    if (!zoomLevelEl) {
      return;
    }
    zoomLevelEl.textContent = `${Math.round(scale * 100)}%`;
  }

  function setSelected(ids) {
    graphState.selectedIDs = new Set(ids);
    applySelectionClasses();
  }

  function applySelectionClasses() {
    for (const [id, g] of graphState.nodeElsByID.entries()) {
      g.classList.toggle("selected", graphState.selectedIDs.has(id));
    }
  }

  function computeLineageIDs(winnerID) {
    graphState.lineageIDs = new Set();
    if (!winnerID || !graphState.byID.has(winnerID)) {
      return;
    }

    const visited = new Set();
    let cursor = winnerID;
    while (cursor && !visited.has(cursor) && graphState.byID.has(cursor)) {
      visited.add(cursor);
      graphState.lineageIDs.add(cursor);
      const cell = graphState.byID.get(cursor);
      if (!cell?.parent_id || !graphState.byID.has(cell.parent_id)) {
        break;
      }
      cursor = cell.parent_id;
    }
  }

  function applyLineageClasses() {
    const hasLineage = graphState.lineageIDs.size > 0;
    for (const [id, nodeEl] of graphState.nodeElsByID.entries()) {
      const inLineage = graphState.lineageIDs.has(id);
      nodeEl.classList.toggle("lineage", inLineage);
      nodeEl.classList.toggle("muted", hasLineage && !graphState.showFullLineage && !inLineage);
    }

    for (const edge of graphState.edges) {
      const edgeEl = graphState.edgeElsByID.get(edge.id);
      if (!edgeEl) {
        continue;
      }
      const inLineage = graphState.lineageIDs.has(edge.source) && graphState.lineageIDs.has(edge.target);
      edgeEl.classList.toggle("lineage-edge", hasLineage && inLineage);
      edgeEl.classList.toggle("muted-edge", hasLineage && !graphState.showFullLineage && !inLineage);
    }
  }

  function renderLineageToggle() {
    if (!lineageToggleEl) {
      return;
    }
    lineageToggleEl.classList.toggle("active", graphState.showFullLineage);
    lineageToggleEl.textContent = graphState.showFullLineage ? "Focus winner lineage" : "Show full lineage";
  }

  async function applyInitialWinnerState() {
    const winnerID = (uiSummary?.winner_cell_id || "").trim();
    if (winnerID && graphState.nodes.has(winnerID)) {
      setSelected([winnerID]);
      await renderCellDetail(winnerID);
      focusWinner(false);
    } else if (allCells.length) {
      const latestID = allCells.at(-1)?.id;
      if (latestID && graphState.nodes.has(latestID)) {
        setSelected([latestID]);
        await renderCellDetail(latestID);
      }
    }

    renderLineageToggle();

    const baselineID = (uiSummary?.baseline_cell_id || "").trim();
    if (!didRunDefaultCompare && winnerID && baselineID && winnerID !== baselineID) {
      didRunDefaultCompare = true;
      setCompareSelections(baselineID, winnerID);
      await renderCompare(baselineID, winnerID);
      setSelected([baselineID, winnerID]);
    }
  }

  async function activateNode(id, isShift) {
    if (isShift) {
      await onShiftClick(id);
      return;
    }

    compareStart = null;
    setSelected([id]);
    await renderCellDetail(id);
  }

  async function onShiftClick(id) {
    if (!compareStart) {
      compareStart = id;
      setSelected([id]);
      panelEl.innerHTML = `<div class="placeholder">Compare mode started at <strong>${escapeHtml(
        id,
      )}</strong>.<br>Shift+click another node.</div>`;
      return;
    }

    const start = compareStart;
    const end = id;
    compareStart = null;
    setSelected([start, end]);
    setCompareSelections(start, end);
    await renderCompare(start, end);
  }

  async function renderCellDetail(id) {
    const resp = await fetch(`/api/cell/${encodeURIComponent(id)}`);
    if (!resp.ok) {
      panelEl.innerHTML = `<div class="placeholder">Unable to load cell ${escapeHtml(id)}.</div>`;
      return;
    }

    const cell = await resp.json();
    const files = cell.files
      .map(
        (file) =>
          `<li><code>${escapeHtml(file.path)}</code> <span class="meta">(${formatBytes(file.size)})</span></li>`,
      )
      .join("");

    panelEl.innerHTML = `
      <h3>${escapeHtml(cell.id)}</h3>
      <div class="kv"><div class="k">Message</div><div class="v">${escapeHtml(cell.message || "")}</div></div>
      <div class="kv"><div class="k">Branch</div><div class="v">${escapeHtml(cell.branch)}</div></div>
      <div class="kv"><div class="k">Timestamp</div><div class="v">${escapeHtml(cell.timestamp)}</div></div>
      <div class="kv"><div class="k">Source</div><div class="v">${escapeHtml(cell.source)}</div></div>
      <div class="kv"><div class="k">Stats</div><div class="v">+${cell.files_added} ~${cell.files_modified} -${cell.files_removed} files | loc ${signed(
        cell.loc_delta,
      )} total ${cell.total_loc}</div></div>
      <div class="kv"><div class="k">Eval</div><div class="v">${evalSummary(cell)}</div></div>
      <div class="kv"><div class="k">Tracked files (${cell.files.length})</div><div class="v"><ul>${files}</ul></div></div>
    `;
  }

  async function renderCompare(cellA, cellB) {
    panelEl.innerHTML = `<div class="placeholder">Loading diff and AI summary for <strong>${escapeHtml(
      cellA,
    )}</strong> and <strong>${escapeHtml(cellB)}</strong>...</div>`;

    const diffResp = await fetch(`/api/diff/${encodeURIComponent(cellA)}/${encodeURIComponent(cellB)}`);
    const diffs = diffResp.ok ? await diffResp.json() : [];

    const blocks = diffs
      .map((diffEntry) => {
        const content = (diffEntry.diff || "")
          .split("\n")
          .map((line) => {
            const safe = escapeHtml(line);
            if (line.startsWith("+")) return `<span class="diff-add">${safe}</span>`;
            if (line.startsWith("-")) return `<span class="diff-del">${safe}</span>`;
            if (line.startsWith("@@")) return `<span class="diff-hunk">${safe}</span>`;
            return safe;
          })
          .join("\n");

        return `
          <div class="diff-block">
            <div class="diff-title">${escapeHtml(diffEntry.status)} ${escapeHtml(diffEntry.path)}</div>
            ${content ? `<pre class="diff-content">${content}</pre>` : ""}
          </div>
        `;
      })
      .join("");

    panelEl.innerHTML = `
      <h3>Compare ${escapeHtml(cellA)} -> ${escapeHtml(cellB)}</h3>
      ${blocks || '<div class="placeholder">No file differences.</div>'}
      <div id="ai-summary" class="placeholder">Generating AI summary...</div>
    `;

    const aiEl = document.getElementById("ai-summary");
    try {
      const compareResp = await fetch("/api/compare", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cell_a: cellA, cell_b: cellB }),
      });
      const result = await compareResp.json();
      if (!compareResp.ok || result.error) {
        aiEl.innerHTML = `<div class="badge warn">AI compare unavailable: ${escapeHtml(
          result.error || compareResp.statusText,
        )}</div>`;
        return;
      }

      const highlights = (result.highlights || []).map((highlight) => `<li>${escapeHtml(highlight)}</li>`).join("");
      aiEl.innerHTML = `
        <h4>AI Summary</h4>
        <p>${escapeHtml(result.summary || "")}</p>
        ${
          result.winner
            ? `<div class="kv"><div class="k">Winner</div><div class="v">${escapeHtml(result.winner)}</div></div>`
            : ""
        }
        ${highlights ? `<ul>${highlights}</ul>` : ""}
      `;
    } catch (_err) {
      aiEl.innerHTML = '<div class="badge bad">AI compare failed.</div>';
    }
  }

  function statusClass(cell) {
    if (cell.tests_failed != null && cell.tests_failed > 0) {
      return "node-fail";
    }
    if ((cell.lint_errors != null && cell.lint_errors > 0) || (cell.type_errors != null && cell.type_errors > 0)) {
      return "node-warn";
    }
    if (cell.tests_passed != null) {
      return "node-pass";
    }
    return "node-none";
  }

  function statusLabel(cell) {
    const cls = statusClass(cell);
    if (cls === "node-pass") return "PASS";
    if (cls === "node-fail") return "FAIL";
    if (cls === "node-warn") return "WARN";
    return "NONE";
  }

  function evalSummary(cell) {
    const parts = [];
    if (cell.tests_passed != null || cell.tests_failed != null) {
      const passed = cell.tests_passed || 0;
      const failed = cell.tests_failed || 0;
      const cls = failed > 0 ? "bad" : "good";
      parts.push(`<span class="badge ${cls}">tests ${passed}/${passed + failed}</span>`);
    }
    if (cell.lint_errors != null) {
      const cls = cell.lint_errors > 0 ? "warn" : "good";
      parts.push(`<span class="badge ${cls}">lint ${cell.lint_errors}</span>`);
    }
    if (cell.type_errors != null) {
      const cls = cell.type_errors > 0 ? "warn" : "good";
      parts.push(`<span class="badge ${cls}">types ${cell.type_errors}</span>`);
    }
    if (!parts.length) {
      return '<span class="badge">not requested</span>';
    }
    return parts.join(" ");
  }

  function svgText(x, y, text, cls, anchor) {
    const node = document.createElementNS(SVG_NS, "text");
    node.setAttribute("x", String(x));
    node.setAttribute("y", String(y));
    node.setAttribute("class", cls || "");
    if (anchor) {
      node.setAttribute("text-anchor", anchor);
    }
    node.textContent = text;
    return node;
  }

  function appendWrappedSvgText(parent, x, y, lines, cls, lineHeight) {
    if (!parent || !lines.length) {
      return;
    }
    for (let i = 0; i < lines.length; i += 1) {
      parent.appendChild(svgText(x, y + i * lineHeight, lines[i], cls));
    }
  }

  function wrapTextLines(input, maxChars, maxLines) {
    const text = String(input || "").trim() || "(no message)";
    if (!text.length) {
      return ["(no message)"];
    }

    const words = text.split(/\s+/);
    const lines = [];
    let current = "";

    for (const word of words) {
      const candidate = current ? `${current} ${word}` : word;
      if (candidate.length <= maxChars) {
        current = candidate;
        continue;
      }

      if (!current) {
        lines.push(word.slice(0, maxChars));
      } else {
        lines.push(current);
        current = word;
      }

      if (lines.length >= maxLines) {
        break;
      }
    }

    if (lines.length < maxLines && current) {
      lines.push(current);
    }

    const consumed = lines.join(" ");
    if (consumed.length < text.length) {
      const last = lines[lines.length - 1] || "";
      const trimmed = last.slice(0, Math.max(0, maxChars - 3)).trimEnd();
      lines[lines.length - 1] = `${trimmed}...`;
    }

    return lines.slice(0, maxLines);
  }

  function pickWinnerCellClient(cells) {
    if (!cells.length) {
      return null;
    }

    const evaluated = cells.filter(
      (cell) => cell.tests_passed != null || cell.tests_failed != null || cell.lint_errors != null || cell.type_errors != null,
    );

    const pool = evaluated.length ? evaluated : cells;
    let best = pool[0];
    for (let i = 1; i < pool.length; i += 1) {
      if (winnerPreferredClient(pool[i], best)) {
        best = pool[i];
      }
    }
    return best;
  }

  function winnerPreferredClient(candidate, current) {
    const candidateFailed = ptrInt(candidate.tests_failed);
    const currentFailed = ptrInt(current.tests_failed);
    if (candidateFailed !== currentFailed) {
      return candidateFailed < currentFailed;
    }

    const candidateLintType = ptrInt(candidate.lint_errors) + ptrInt(candidate.type_errors);
    const currentLintType = ptrInt(current.lint_errors) + ptrInt(current.type_errors);
    if (candidateLintType !== currentLintType) {
      return candidateLintType < currentLintType;
    }

    const candidatePassed = ptrInt(candidate.tests_passed);
    const currentPassed = ptrInt(current.tests_passed);
    if (candidatePassed !== currentPassed) {
      return candidatePassed > currentPassed;
    }

    return (candidate.sequence || 0) > (current.sequence || 0);
  }

  function calculatePassRateClient(cells) {
    let evaluated = 0;
    let passed = 0;
    for (const cell of cells) {
      if (cell.tests_passed == null && cell.tests_failed == null) {
        continue;
      }
      evaluated += 1;
      if (ptrInt(cell.tests_failed) === 0) {
        passed += 1;
      }
    }
    if (!evaluated) {
      return 0;
    }
    return (passed / evaluated) * 100;
  }

  function countForkPointsClient(cells) {
    const childCountByParent = new Map();
    for (const cell of cells) {
      if (!cell.parent_id) {
        continue;
      }
      childCountByParent.set(cell.parent_id, (childCountByParent.get(cell.parent_id) || 0) + 1);
    }

    let forks = 0;
    for (const count of childCountByParent.values()) {
      if (count > 1) {
        forks += 1;
      }
    }
    return forks;
  }

  function cellByID(id) {
    if (!id) {
      return null;
    }
    return allCells.find((cell) => cell.id === id) || null;
  }

  function formatPercent(value) {
    if (!Number.isFinite(value)) {
      return "0%";
    }
    return `${value.toFixed(1)}%`;
  }

  function ptrInt(value) {
    if (value == null) {
      return 0;
    }
    return Number(value) || 0;
  }

  function signed(n) {
    return n >= 0 ? `+${n}` : `${n}`;
  }

  function truncate(input, maxLen) {
    const value = String(input || "");
    if (value.length <= maxLen) {
      return value;
    }
    return `${value.slice(0, maxLen - 3)}...`;
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }

  function escapeHtml(input) {
    const s = String(input || "");
    return s
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  init().catch((err) => {
    panelEl.innerHTML = `<div class="badge bad">UI failed to initialize: ${escapeHtml(err.message || err)}</div>`;
  });
})();
