(() => {
  const root = document.querySelector("[data-ai-legitimacy]");
  const status = document.querySelector("[data-ai-legitimacy-status]");
  if (!root) return;

  const setStatus = (message, error = false) => {
    if (!status) return;
    status.textContent = message;
    status.className = `mt-3 text-xs ${error ? "text-destructive" : "text-muted-foreground"}`;
  };

  const t = (key, params) => (window.goferT ? window.goferT(key, params) : key);
  const tn = (key, count, params) => (window.goferTn ? window.goferTn(key, count, params) : key);

  const toggle = (labelText, checked) => {
    const label = document.createElement("label");
    label.className = "flex items-center gap-1.5 text-xs";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = Boolean(checked);
    input.className = "h-3.5 w-3.5 rounded border-border";
    label.append(input, document.createTextNode(labelText));
    return { label, input };
  };

  const save = async (profile, vip, allowlisted, blocked) => {
    try {
      const response = await fetch(`/api/ai/legitimacy/${encodeURIComponent(profile.SenderEmail)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ vip, allowlisted, blocked }),
      });
      if (!response.ok) throw new Error(await response.text());
      setStatus(t("js.ai.legitimacy.updated", { email: profile.SenderEmail }));
      await load();
    } catch (error) {
      setStatus(error.message || t("js.ai.legitimacy.update_failed"), true);
    }
  };

  const row = (profile) => {
    const item = document.createElement("div");
    item.className = "py-3 space-y-2";
    const head = document.createElement("div");
    head.className = "flex flex-wrap items-center justify-between gap-2";
    const email = document.createElement("span");
    email.className = "truncate text-sm font-medium";
    email.textContent = profile.SenderEmail;
    const meta = document.createElement("span");
    meta.className = "text-xs text-muted-foreground";
    meta.textContent = t("js.ai.legitimacy.summary", {
      state: profile.State,
      messages: tn("js.ai.legitimacy.messages", profile.MessageCount),
      replies: tn("js.ai.legitimacy.replies", profile.ReplyCount),
    });
    head.append(email, meta);
    const controls = document.createElement("div");
    controls.className = "flex flex-wrap items-center gap-4";
    const vip = toggle(t("js.ai.legitimacy.vip"), profile.VIP);
    const trusted = toggle(t("js.ai.legitimacy.trusted"), profile.Allowlisted);
    const blocked = toggle(t("js.ai.legitimacy.blocked"), profile.Blocked);
    const apply = () => save(profile, vip.input.checked, trusted.input.checked, blocked.input.checked);
    vip.input.addEventListener("change", apply);
    trusted.input.addEventListener("change", () => {
      if (trusted.input.checked) blocked.input.checked = false;
      apply();
    });
    blocked.input.addEventListener("change", () => {
      if (blocked.input.checked) trusted.input.checked = false;
      apply();
    });
    controls.append(vip.label, trusted.label, blocked.label);
    item.append(head, controls);
    return item;
  };

  const load = async () => {
    try {
      const response = await fetch("/api/ai/legitimacy");
      if (!response.ok) throw new Error(await response.text());
      const payload = await response.json();
      root.replaceChildren();
      const profiles = payload.profiles || [];
      if (profiles.length === 0) {
        const empty = document.createElement("p");
        empty.className = "py-3 text-xs text-muted-foreground";
        empty.textContent = t("js.ai.legitimacy.empty");
        root.append(empty);
        return;
      }
      profiles.forEach((profile) => root.append(row(profile)));
    } catch (error) {
      setStatus(error.message || t("js.ai.legitimacy.load_failed"), true);
    }
  };

  load();
})();
