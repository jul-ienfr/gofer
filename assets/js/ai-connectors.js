(() => {
  const root = document.querySelector("[data-ai-connectors]");
  const form = document.querySelector("[data-ai-connector-form]");
  const status = document.querySelector("[data-ai-connectors-status]");
  const deliveries = document.querySelector("[data-ai-deliveries]");
  if (!root) return;

  const setStatus = (message, error = false) => {
    if (!status) return;
    status.textContent = message;
    status.className = `mt-3 text-xs ${error ? "text-destructive" : "text-muted-foreground"}`;
  };

  const row = (connector) => {
    const item = document.createElement("div");
    item.className = "flex flex-wrap items-center justify-between gap-3 py-3";
    const info = document.createElement("div");
    info.className = "min-w-0";
    const title = document.createElement("p");
    title.className = "truncate text-sm font-medium";
    title.textContent = connector.name;
    const url = document.createElement("p");
    url.className = "truncate text-xs text-muted-foreground";
    url.textContent = `${connector.type} · ${connector.url}`;
    info.append(title, url);
    const actions = document.createElement("div");
    actions.className = "flex items-center gap-3";
    const toggle = document.createElement("label");
    toggle.className = "flex items-center gap-2 text-xs";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = Boolean(connector.enabled);
    checkbox.className = "h-4 w-4 rounded border-border";
    checkbox.addEventListener("change", async () => {
      try {
        const response = await fetch(`/api/ai/connectors/${encodeURIComponent(connector.id)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled: checkbox.checked }),
        });
        if (!response.ok) throw new Error(await response.text());
        setStatus("Connector updated.");
      } catch (error) {
        checkbox.checked = !checkbox.checked;
        setStatus(error.message || "Could not update connector.", true);
      }
    });
    toggle.append(checkbox, document.createTextNode("Enabled"));
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "rounded-md border border-border px-2 py-1 text-xs hover:bg-muted";
    remove.textContent = "Delete";
    remove.addEventListener("click", async () => {
      if (!confirm(`Delete connector “${connector.name}”?`)) return;
      try {
        const response = await fetch(`/api/ai/connectors/${encodeURIComponent(connector.id)}`, { method: "DELETE" });
        if (!response.ok) throw new Error(await response.text());
        item.remove();
        setStatus("Connector deleted.");
      } catch (error) {
        setStatus(error.message || "Could not delete connector.", true);
      }
    });
    actions.append(toggle, remove);
    item.append(info, actions);
    return item;
  };

  const loadDeliveries = async () => {
    if (!deliveries) return;
    try {
      const response = await fetch("/api/ai/routing");
      if (!response.ok) throw new Error(await response.text());
      const payload = await response.json();
      deliveries.replaceChildren();
      const items = payload.deliveries || [];
      const unsubscribes = payload.unsubscribes || [];
      const actions = payload.actions || [];
      if (items.length === 0 && unsubscribes.length === 0 && actions.length === 0) {
        const empty = document.createElement("p");
        empty.className = "py-3 text-xs text-muted-foreground";
        empty.textContent = "No automation activity yet.";
        deliveries.append(empty);
        return;
      }
      items.forEach((item) => {
        const line = document.createElement("div");
        line.className = "py-2 text-xs";
        const head = document.createElement("div");
        head.className = "flex flex-wrap items-center justify-between gap-2";
        const label = document.createElement("span");
        label.className = "font-medium";
        label.textContent = `${item.Project || "project"} → ${item.Connector || "connector"}`;
        const badge = document.createElement("span");
        badge.className = "text-muted-foreground";
        badge.textContent = `${item.State} · ${item.Attempts} attempt(s)`;
        head.append(label, badge);
        line.append(head);
        if (item.LastError) {
          const error = document.createElement("p");
          error.className = "mt-1 text-red-700 dark:text-red-300";
          error.textContent = item.LastError;
          line.append(error);
        }
        deliveries.append(line);
      });
      actions.forEach((item) => {
        const line = document.createElement("div");
        line.className = "py-2 text-xs";
        const head = document.createElement("div");
        head.className = "flex flex-wrap items-center justify-between gap-2";
        const label = document.createElement("span");
        label.className = "font-medium";
        label.textContent = `${item.Kind === "move_junk" ? "Moved to Junk" : item.Kind === "route_move" ? "Route move" : item.Kind} · message #${item.MessageID}`;
        const right = document.createElement("span");
        right.className = "flex items-center gap-2 text-muted-foreground";
        const badge = document.createElement("span");
        badge.textContent = item.State;
        right.append(badge);
        const deadline = Date.parse(item.UndoDeadline || "");
        if (item.State === "executed" && Number.isFinite(deadline) && deadline > Date.now()) {
          const undo = document.createElement("button");
          undo.type = "button";
          undo.className = "rounded-md border border-border px-2 py-1 hover:bg-muted";
          undo.textContent = "Undo";
          undo.addEventListener("click", async () => {
            undo.disabled = true;
            try {
              const response = await fetch(`/api/ai/automation/${encodeURIComponent(item.ID)}/undo`, { method: "POST" });
              if (!response.ok) throw new Error(await response.text());
              setStatus("Automation undone.");
              await loadDeliveries();
            } catch (error) {
              setStatus(error.message || "Could not undo automation.", true);
              undo.disabled = false;
            }
          });
          right.append(undo);
        }
        head.append(label, right);
        line.append(head);
        deliveries.append(line);
      });
      unsubscribes.forEach((item) => {
        const line = document.createElement("div");
        line.className = "py-2 text-xs";
        const head = document.createElement("div");
        head.className = "flex flex-wrap items-center justify-between gap-2";
        const label = document.createElement("span");
        label.className = "font-medium";
        label.textContent = `Unsubscribe · message #${item.MessageID}`;
        const badge = document.createElement("span");
        badge.className = "text-muted-foreground";
        badge.textContent = `${item.Status}${item.HTTPStatus ? ` · HTTP ${item.HTTPStatus}` : ""}`;
        head.append(label, badge);
        line.append(head);
        if (item.Error) {
          const error = document.createElement("p");
          error.className = "mt-1 text-red-700 dark:text-red-300";
          error.textContent = item.Error;
          line.append(error);
        }
        deliveries.append(line);
      });
    } catch (error) {
      deliveries.textContent = "Could not load deliveries.";
    }
  };

  const load = async () => {
    try {
      const response = await fetch("/api/ai/connectors");
      if (!response.ok) throw new Error(await response.text());
      const payload = await response.json();
      root.replaceChildren();
      if (!payload.connectors || payload.connectors.length === 0) {
        const empty = document.createElement("p");
        empty.className = "py-3 text-xs text-muted-foreground";
        empty.textContent = "No routing connector yet.";
        root.append(empty);
        return;
      }
      payload.connectors.forEach((connector) => root.append(row(connector)));
    } catch (error) {
      setStatus(error.message || "Could not load connectors.", true);
    }
  };

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      const response = await fetch("/api/ai/connectors", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: String(data.get("name") || ""),
          type: "webhook",
          url: String(data.get("url") || ""),
          secret: String(data.get("secret") || ""),
          allow_private: data.get("allow_private") === "on",
          enabled: true,
        }),
      });
      if (!response.ok) throw new Error(await response.text());
      form.reset();
      setStatus("Connector created.");
      await load();
    } catch (error) {
      setStatus(error.message || "Could not create connector.", true);
    }
  });

  load();
  loadDeliveries();
})();
