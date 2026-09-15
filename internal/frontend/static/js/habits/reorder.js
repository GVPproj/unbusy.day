const TAP_SLOP = 4;

function rowsIn(matrix) {
  return [...matrix.querySelectorAll("#habit-grid tbody > tr[data-sort-order]")];
}

function idOf(row) {
  return Number(row.id.slice("habit-".length));
}

function idsIn(matrix) {
  return rowsIn(matrix).map(idOf);
}

function sameOrder(a, b) {
  return a.length === b.length && a.every((id, index) => id === b[index]);
}

function labelOf(row) {
  return row.querySelector(".habit-name")?.textContent.trim() || "Habit";
}

function canReorder(matrix) {
  return matrix.querySelector("#habit-grid")?.dataset.reorderable === "true";
}

function restoreFocusAfterMorph(matrix, id) {
  const document = matrix.ownerDocument;
  const Observer = document.defaultView.MutationObserver;
  const refocus = () => {
    const active = document.activeElement;
    if (active && active !== document.body) return;
    document.getElementById(`habit-${id}`)?.focus({ preventScroll: true });
  };
  const observer = new Observer(refocus);
  observer.observe(matrix, { childList: true, subtree: true });
  setTimeout(() => observer.disconnect(), 1000);
}

export function commitHabitOrder(matrix, row, from, announce, refocus = true) {
  const rows = rowsIn(matrix);
  const to = rows.map(idOf);
  if (sameOrder(from, to) || !row.parentElement || !rows.includes(row)) return false;

  const order = rows.map((candidate, sortOrder) => {
    candidate.dataset.sortOrder = String(sortOrder);
    return { id: idOf(candidate), sortOrder };
  });
  const position = rows.indexOf(row) + 1;
  if (announce) announce(`Moved ${labelOf(row)} to position ${position} of ${rows.length}.`);
  if (refocus) {
    row.focus({ preventScroll: true });
    restoreFocusAfterMorph(matrix, idOf(row));
  }

  // Keep the dropped layout visible until the server confirms or rejects it.
  const CustomEvent = matrix.ownerDocument.defaultView.CustomEvent;
  matrix.dispatchEvent(new CustomEvent("reorder", { detail: { order } }));
  return true;
}

function bindArb(name, handle) {
  if (!handle || typeof handle.isActive !== "function" || typeof handle.cancel !== "function") {
    throw new Error(`habit-reorder: ${name} path returned no arbitration handle`);
  }
  return handle;
}

function placeAt(tbody, row, index) {
  const others = [...tbody.children].filter((candidate) => candidate !== row);
  tbody.insertBefore(row, others[index] || null);
}

function restoreRows(gesture) {
  const byID = new Map([...gesture.tbody.children].map((row) => [idOf(row), row]));
  for (const id of gesture.from) {
    const row = byID.get(id);
    if (row) gesture.tbody.append(row);
  }
}

function initPointer({ matrix, announce }, arb) {
  const document = matrix.ownerDocument;
  let gesture = null;

  function isActive() {
    return gesture !== null;
  }

  function stopWatching(current) {
    current.observer.disconnect();
    matrix.classList.remove("reordering");
    if (matrix.hasPointerCapture?.(current.pointerId)) matrix.releasePointerCapture(current.pointerId);
    document.removeEventListener("datastar-fetch", current.onPatch, true);
    document.removeEventListener("beforetoggle", current.onDialog, true);
    document.removeEventListener("toggle", current.onDialog, true);
  }

  function cancel() {
    if (!gesture) return;
    const current = gesture;
    gesture = null;
    stopWatching(current);
    current.row.classList.remove("dragging");
    if (!current.serverChanged && current.row.parentElement === current.tbody) restoreRows(current);
  }

  function watchServer(current) {
    current.serverChanged = false;
    current.onPatch = (event) => {
      if (event.detail?.type !== "datastar-patch-elements") return;
      const elements = event.detail.argsRaw?.elements;
      if (typeof elements !== "string") return;
      const patch = document.createElement("template");
      patch.innerHTML = elements;
      if (patch.content.querySelector("#habit-grid")) {
        current.serverChanged = true;
        cancel();
      }
    };
    current.onDialog = (event) => {
      if (event.target?.matches?.("dialog") && (event.type === "beforetoggle" || event.target.open)) cancel();
    };
    document.addEventListener("datastar-fetch", current.onPatch, true);
    document.addEventListener("beforetoggle", current.onDialog, true);
    document.addEventListener("toggle", current.onDialog, true);

    const Observer = document.defaultView.MutationObserver;
    current.observer = new Observer((records) => {
      const changed = records.some((record) => {
        if (record.type === "attributes") return record.target.matches?.("tr[data-sort-order]");
        return [...record.addedNodes, ...record.removedNodes].some((node) =>
          node.nodeType === 1 && (node.matches?.("tr[data-sort-order]") || node.querySelector?.("tr[data-sort-order]")));
      });
      if (changed) {
        current.serverChanged = true;
        cancel();
      }
    });
    const grid = matrix.querySelector("#habit-grid");
    if (grid) current.observer.observe(grid, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-sort-order"],
    });
  }

  function onPointerdown(event) {
    if (gesture || !canReorder(matrix) || event.button !== 0 || event.isPrimary === false || event.target.closest?.(".habit-action")) return;
    const header = event.target.closest?.("tbody th[scope='row']");
    const row = header?.closest("tr[data-sort-order]");
    if (!row || !rowsIn(matrix).includes(row)) return;
    arb.keyboard?.cancel();
    event.preventDefault();
    event.stopPropagation();
    // Capture on the stable container: reparenting a captured row can lose capture.
    matrix.setPointerCapture?.(event.pointerId);
    matrix.classList.add("reordering");
    gesture = {
      row,
      tbody: row.parentElement,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      from: idsIn(matrix),
      moved: false,
    };
    watchServer(gesture);
  }

  function onPointermove(event) {
    const current = gesture;
    if (!current || event.pointerId !== current.pointerId) return;
    const dx = event.clientX - current.startX;
    const dy = event.clientY - current.startY;
    if (!current.moved && Math.abs(dx) <= TAP_SLOP && Math.abs(dy) <= TAP_SLOP) return;
    if (!current.moved) {
      current.moved = true;
      current.row.classList.add("dragging");
    }
    event.preventDefault();
    const otherRows = rowsIn(matrix).filter((row) => row !== current.row);
    const before = otherRows.find((row) => {
      const rect = row.getBoundingClientRect();
      return event.clientY < rect.top + rect.height / 2;
    });
    current.tbody.insertBefore(current.row, before || null);
    current.observer.takeRecords();
  }

  function finish(event, shouldCommit) {
    const current = gesture;
    if (!current || event.pointerId !== current.pointerId) return;
    gesture = null;
    stopWatching(current);
    current.row.classList.remove("dragging");
    if (!shouldCommit || !current.moved) {
      if (!current.serverChanged) restoreRows(current);
      return;
    }
    commitHabitOrder(matrix, current.row, current.from, announce);
  }

  matrix.addEventListener("pointerdown", onPointerdown);
  matrix.addEventListener("pointermove", onPointermove);
  matrix.addEventListener("pointerup", (event) => finish(event, true));
  matrix.addEventListener("pointercancel", (event) => finish(event, false));
  matrix.addEventListener("lostpointercapture", (event) => finish(event, false));
  return { isActive, cancel };
}

function initKeyboard({ matrix, announce }, arb) {
  let active = null;

  function isActive() {
    return active !== null;
  }

  function cancel() {
    if (!active) return;
    restoreRows(active);
    active = null;
  }

  function onKeydown(event) {
    if (!event.altKey || !canReorder(matrix) || arb.pointer?.isActive()) return;
    const row = event.target.closest?.("tr[data-sort-order]");
    if (!row || event.target !== row) return;
    const rows = rowsIn(matrix);
    const fromIndex = rows.indexOf(row);
    if (fromIndex < 0) return;
    let toIndex = fromIndex;
    if (event.key === "ArrowUp") toIndex = Math.max(0, fromIndex - 1);
    else if (event.key === "ArrowDown") toIndex = Math.min(rows.length - 1, fromIndex + 1);
    else if (event.key === "Home") toIndex = 0;
    else if (event.key === "End") toIndex = rows.length - 1;
    else return;
    event.preventDefault();
    if (toIndex === fromIndex) return;

    active = { row, tbody: row.parentElement, from: rows.map(idOf) };
    placeAt(active.tbody, row, toIndex);
    const current = active;
    active = null;
    commitHabitOrder(matrix, row, current.from, announce);
    row.scrollIntoView?.({ block: "nearest" });
  }

  matrix.addEventListener("keydown", onKeydown);
  return { isActive, cancel };
}

export function initHabitReorder(matrix, announce) {
  const ctx = { matrix, announce };
  const arb = {};
  arb.keyboard = bindArb("keyboard", initKeyboard(ctx, arb));
  arb.pointer = bindArb("pointer", initPointer(ctx, arb));
  return arb;
}

if (typeof window !== "undefined") {
  const matrix = window.document.getElementById("habit-matrix");
  const announcer = window.document.getElementById("sr-announce");
  if (matrix) initHabitReorder(matrix, (message) => {
    if (announcer) announcer.textContent = message;
  });
}
