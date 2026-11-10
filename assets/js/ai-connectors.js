(() => {
  const root = document.querySelector("[data-ai-connectors]");
  const form = document.querySelector("[data-ai-connector-form]");
  const status = document.querySelector("[data-ai-connectors-status]");
  const deliveries = document.querySelector("[data-ai-deliveries]");
  if (!root) return;

  const t = (key, params) => (window.goferT ? window.goferT(key, params) : key);
  const tn = (key, count, params) => (window.goferTn ? window.goferTn(key, count, params) : key);
  const stateLabel = (state) => {
    if (!state) return "";
    const key = "js.ai.state." + String(state).toLowerCase();
    const label = t(key);
    return label === key ? state : label;
  };

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
        setStatus(t("js.ai.connectors.updated"));
      } catch (error) {
        checkbox.checked = !checkbox.checked;
        setStatus(error.message || t("js.ai.connectors.update_failed"), true);
      }
    });
    toggle.append(checkbox, document.createTextNode(t("js.ai.connectors.enabled")));
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "rounded-md border border-border px-2 py-1 text-xs hover:bg-muted";
    remove.textContent = t("js.ai.connectors.delete");
    remove.addEventListener("click", async () => {
      if (!confirm(t("js.ai.connectors.delete_confirm", { name: connector.name }))) return;
      try {
        const response = await fetch(`/api/ai/connectors/${encodeURIComponent(connector.id)}`, { method: "DELETE" });
        if (!response.ok) throw new Error(await response.text());
        item.remove();
        setStatus(t("js.ai.connectors.deleted"));
      } catch (error) {
        setStatus(error.message || t("js.ai.connectors.delete_failed"), true);
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
        empty.textContent = t("js.ai.connectors.empty_activity");
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
        label.textContent = `${item.Project || t("js.ai.connectors.project_fallback")} → ${item.Connector || t("js.ai.connectors.connector_fallback")}`;
        const badge = document.createElement("span");
        badge.className = "text-muted-foreground";
        badge.textContent = t("js.ai.connectors.summary", {
          state: stateLabel(item.State),
          attempts: tn("js.ai.connectors.attempts", item.Attempts),
        });
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
        label.textContent = `${item.Kind === "move_junk" ? t("js.ai.connectors.moved_junk") : item.Kind === "route_move" ? t("js.ai.connectors.route_move") : item.Kind} · ${t("js.ai.connectors.message_ref", { id: item.MessageID })}`;
        const right = document.createElement("span");
        right.className = "flex items-center gap-2 text-muted-foreground";
        const badge = document.createElement("span");
        badge.textContent = stateLabel(item.State);
        right.append(badge);
        const deadline = Date.parse(item.UndoDeadline || "");
        if (item.State === "executed" && Number.isFinite(deadline) && deadline > Date.now()) {
          const undo = document.createElement("button");
          undo.type = "button";
          undo.className = "rounded-md border border-border px-2 py-1 hover:bg-muted";
          undo.textContent = t("js.ai.connectors.undo");
          undo.addEventListener("click", async () => {
            undo.disabled = true;
            try {
              const response = await fetch(`/api/ai/automation/${encodeURIComponent(item.ID)}/undo`, { method: "POST" });
              if (!response.ok) throw new Error(await response.text());
              setStatus(t("js.ai.connectors.undone"));
              await loadDeliveries();
            } catch (error) {
              setStatus(error.message || t("js.ai.connectors.undo_failed"), true);
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
        label.textContent = t("js.ai.connectors.unsubscribe", { id: item.MessageID });
        const badge = document.createElement("span");
        badge.className = "text-muted-foreground";
        badge.textContent = `${stateLabel(item.Status)}${item.HTTPStatus ? ` · HTTP ${item.HTTPStatus}` : ""}`;
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
      deliveries.textContent = t("js.ai.connectors.deliveries_failed");
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
        empty.textContent = t("js.ai.connectors.empty");
        root.append(empty);
        return;
      }
      payload.connectors.forEach((connector) => root.append(row(connector)));
    } catch (error) {
      setStatus(error.message || t("js.ai.connectors.load_failed"), true);
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
      setStatus(t("js.ai.connectors.created"));
      await load();
    } catch (error) {
      setStatus(error.message || t("js.ai.connectors.create_failed"), true);
    }
  });

  load();
  loadDeliveries();
})();
