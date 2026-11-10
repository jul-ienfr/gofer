(function () {
  "use strict"

  var root = document.querySelector("[data-ai-projects]")
  if (!root) return
  var status = document.querySelector("[data-ai-projects-status]")
  var seed = document.querySelector("[data-ai-projects-seed]")

  function setStatus(message, isError) {
    if (!status) return
    status.textContent = message || ""
    status.classList.toggle("text-red-700", !!isError)
    status.classList.toggle("dark:text-red-300", !!isError)
  }

  function t(key, params) {
    return window.goferT ? window.goferT(key, params) : key
  }

  function checkbox(labelText, checked) {
    var label = document.createElement("label")
    label.className = "inline-flex items-center gap-2 text-xs text-foreground"
    var input = document.createElement("input")
    input.type = "checkbox"
    input.checked = !!checked
    input.className = "size-4 rounded border-border"
    label.appendChild(input)
    label.appendChild(document.createTextNode(labelText))
    return { label: label, input: input }
  }

  function field(labelText, type, placeholder) {
    var label = document.createElement("label")
    label.className = "text-xs font-medium text-foreground"
    label.textContent = labelText
    var input = document.createElement("input")
    input.type = type
    input.placeholder = placeholder || ""
    input.className = "mt-1 w-full rounded-md border border-border bg-background px-2 py-1 text-xs"
    label.appendChild(input)
    return { label: label, input: input }
  }

  function routesPanel(project) {
    var details = document.createElement("details")
    details.className = "text-xs"
    var summary = document.createElement("summary")
    summary.className = "cursor-pointer text-muted-foreground hover:text-foreground"
    summary.textContent = t("js.ai.projects.routes")
    details.appendChild(summary)
    var body = document.createElement("div")
    body.className = "mt-2 space-y-2 border-l border-border pl-3"
    details.appendChild(body)

    function loadRoutes() {
      body.textContent = t("js.ai.projects.loading_routes")
      fetch("/api/ai/projects/" + encodeURIComponent(project.ID) + "/routes", { credentials: "same-origin" })
        .then(function (response) {
          if (!response.ok) throw new Error("load failed")
          return response.json()
        })
        .then(function (data) {
          body.textContent = ""
          var routes = Array.isArray(data.routes) ? data.routes : []
          if (!routes.length) {
            var empty = document.createElement("p")
            empty.className = "text-muted-foreground"
            empty.textContent = t("js.ai.projects.no_routes")
            body.appendChild(empty)
          }
          routes.forEach(function (route) {
            var line = document.createElement("div")
            line.className = "flex items-center justify-between gap-2"
            var text = document.createElement("span")
            var actionText = ""
            try {
              actionText = (JSON.parse(route.ActionsJSON || "[]") || [])
                .map(function (action) { return action.kind + (action.value ? ":" + action.value : "") })
                .join(", ")
            } catch (_) {}
            var textParts = [t("js.ai.projects.route_meta", { name: route.Name, priority: route.Priority })]
            if (actionText) textParts.push(actionText)
            if (!route.Enabled) textParts.push(t("js.ai.projects.disabled"))
            text.textContent = textParts.join(" · ")
            var remove = document.createElement("button")
            remove.type = "button"
            remove.className = "rounded border border-border px-2 py-0.5 hover:bg-muted"
            remove.textContent = t("js.ai.projects.delete")
            remove.addEventListener("click", function () {
              fetch("/api/ai/routes/" + encodeURIComponent(route.ID), {
                method: "DELETE",
                credentials: "same-origin",
              }).then(function (response) {
                if (!response.ok) throw new Error("delete failed")
                setStatus(t("js.ai.projects.route_deleted"), false)
                loadRoutes()
              }).catch(function () {
                setStatus(t("js.ai.projects.route_delete_failed"), true)
              })
            })
            line.appendChild(text)
            line.appendChild(remove)
            body.appendChild(line)
          })
          body.appendChild(addForm())
        })
        .catch(function () {
          body.textContent = t("js.ai.projects.routes_failed")
        })
    }

    function addForm() {
      var form = document.createElement("div")
      form.className = "space-y-2 pt-2"
      var name = field(t("js.ai.projects.route_name"), "text", t("js.ai.projects.route_name_placeholder"))
      var sender = field(t("js.ai.projects.sender_email"), "email", "recruiter@example.com")
      var kindLabel = document.createElement("label")
      kindLabel.className = "text-xs font-medium text-foreground"
      kindLabel.textContent = t("js.ai.projects.action")
      var kind = document.createElement("select")
      kind.className = "mt-1 w-full rounded-md border border-border bg-background px-2 py-1 text-xs"
      ;[["webhook", t("js.ai.projects.webhook")], ["label", t("js.ai.projects.label")], ["move", t("js.ai.projects.move_folder")]].forEach(function (item) {
        var option = document.createElement("option")
        option.value = item[0]
        option.textContent = item[1]
        kind.appendChild(option)
      })
      kindLabel.appendChild(kind)
      var connectorLabel = document.createElement("label")
      connectorLabel.className = "text-xs font-medium text-foreground"
      connectorLabel.textContent = t("js.ai.projects.webhook_connector")
      var select = document.createElement("select")
      select.className = "mt-1 w-full rounded-md border border-border bg-background px-2 py-1 text-xs"
      var placeholder = document.createElement("option")
      placeholder.value = ""
      placeholder.textContent = t("js.ai.projects.loading_connectors")
      select.appendChild(placeholder)
      connectorLabel.appendChild(select)
      var targetLabel = document.createElement("label")
      targetLabel.className = "hidden text-xs font-medium text-foreground"
      targetLabel.textContent = t("js.ai.projects.target")
      var target = document.createElement("input")
      target.type = "text"
      target.className = "mt-1 w-full rounded-md border border-border bg-background px-2 py-1 text-xs"
      target.placeholder = t("js.ai.projects.target_placeholder")
      targetLabel.appendChild(target)
      var add = document.createElement("button")
      add.type = "button"
      add.className = "rounded-md border border-border px-2 py-1 text-xs font-medium hover:bg-muted"
      add.textContent = t("js.ai.projects.add_route")
      form.appendChild(name.label)
      form.appendChild(sender.label)
      form.appendChild(kindLabel)
      form.appendChild(connectorLabel)
      form.appendChild(targetLabel)
      form.appendChild(add)
      kind.addEventListener("change", function () {
        var webhook = kind.value === "webhook"
        connectorLabel.classList.toggle("hidden", !webhook)
        targetLabel.classList.toggle("hidden", webhook)
      })
      fetch("/api/ai/connectors", { credentials: "same-origin" })
        .then(function (response) { return response.json() })
        .then(function (data) {
          select.textContent = ""
          var list = Array.isArray(data.connectors) ? data.connectors : []
          if (!list.length) {
            placeholder.textContent = t("js.ai.projects.add_connector_first")
            select.appendChild(placeholder)
            return
          }
          list.forEach(function (connector) {
            var option = document.createElement("option")
            option.value = connector.id
            option.textContent = connector.name
            select.appendChild(option)
          })
        })
        .catch(function () {})
      add.addEventListener("click", function () {
        var value = kind.value === "webhook" ? select.value : target.value.trim()
        if (!sender.input.value || !value) {
          setStatus(kind.value === "webhook" ? t("js.ai.projects.need_sender_connector") : t("js.ai.projects.need_sender_target"), true)
          return
        }
        add.disabled = true
        fetch("/api/ai/projects/" + encodeURIComponent(project.ID) + "/routes", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "same-origin",
          body: JSON.stringify({
            name: name.input.value || t("js.ai.projects.default_route_name"),
            enabled: true,
            priority: 10,
            match_json: JSON.stringify({ senders: [sender.input.value.trim().toLowerCase()] }),
            actions_json: JSON.stringify([{ kind: kind.value, value: value }]),
          }),
        })
          .then(function (response) {
            if (!response.ok) throw new Error("create failed")
            setStatus(kind.value === "webhook" ? t("js.ai.projects.route_created_external") : t("js.ai.projects.route_created"), false)
            loadRoutes()
          })
          .catch(function () {
            setStatus(t("js.ai.projects.route_create_failed"), true)
            add.disabled = false
          })
      })
      return form
    }

    details.addEventListener("toggle", function () {
      if (details.open) loadRoutes()
    })
    return details
  }

  function render(projects) {
    root.textContent = ""
    if (!projects.length) {
      var empty = document.createElement("p")
      empty.className = "py-3 text-xs text-muted-foreground"
      empty.textContent = t("js.ai.projects.empty")
      root.appendChild(empty)
      return
    }
    projects.forEach(function (project) {
      var row = document.createElement("div")
      row.className = "py-3 space-y-2"
      var heading = document.createElement("div")
      heading.className = "flex flex-wrap items-center justify-between gap-2"
      var title = document.createElement("div")
      title.className = "text-sm font-medium text-foreground"
      title.textContent = project.Name
      var slug = document.createElement("span")
      slug.className = "ml-2 text-xs text-muted-foreground"
      slug.textContent = project.Slug + " · " + (project.DataClassification || t("js.ai.projects.internal"))
      title.appendChild(slug)
      heading.appendChild(title)
      row.appendChild(heading)
      var options = document.createElement("div")
      options.className = "flex flex-wrap items-center gap-4"
      var routing = checkbox(t("js.ai.projects.external_routing"), project.AllowExternalRouting)
      var drafting = checkbox(t("js.ai.projects.llm_draft"), project.AllowLLMDraft !== false)
      var spam = checkbox(t("js.ai.projects.auto_spam"), project.AllowAutoSpam)
      var classificationLabel = document.createElement("label")
      classificationLabel.className = "text-xs font-medium text-foreground"
      classificationLabel.textContent = t("js.ai.projects.data")
      var classification = document.createElement("select")
      classification.className = "ml-1 rounded-md border border-border bg-background px-2 py-1 text-xs"
      ;[["public", t("js.ai.projects.public")], ["internal", t("js.ai.projects.internal_label")], ["confidential", t("js.ai.projects.confidential")], ["restricted", t("js.ai.projects.restricted")]].forEach(function (item) {
        var option = document.createElement("option")
        option.value = item[0]
        option.textContent = item[1]
        classification.appendChild(option)
      })
      classification.value = ["public", "internal", "confidential", "restricted"].indexOf(project.DataClassification) >= 0 ? project.DataClassification : "internal"
      classificationLabel.appendChild(classification)
      var syncClassification = function () {
        var external = classification.value === "public" || classification.value === "internal"
        routing.input.disabled = !external
        if (!external) routing.input.checked = false
      }
      classification.addEventListener("change", syncClassification)
      syncClassification()
      options.appendChild(routing.label)
      options.appendChild(drafting.label)
      options.appendChild(spam.label)
      options.appendChild(classificationLabel)
      var save = document.createElement("button")
      save.type = "button"
      save.className = "ml-auto rounded-md border border-border px-2 py-1 text-xs font-medium hover:bg-muted"
      save.textContent = t("js.ai.projects.save")
      save.addEventListener("click", function () {
        save.disabled = true
        setStatus(t("js.ai.projects.saving", { name: project.Name }), false)
        fetch("/api/ai/projects/" + encodeURIComponent(project.ID), {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          credentials: "same-origin",
          body: JSON.stringify({
            allow_external_routing: routing.input.checked,
            allow_llm_draft: drafting.input.checked,
            allow_auto_spam: spam.input.checked,
            data_classification: classification.value,
          }),
        })
          .then(function (response) {
            if (!response.ok) throw new Error(t("js.settings.status.save_failed"))
            return response.json()
          })
          .then(function () {
            setStatus(t("js.ai.projects.saved"), false)
          })
          .catch(function () {
            setStatus(t("js.ai.projects.save_failed"), true)
          })
          .then(function () {
            save.disabled = false
          })
      })
      options.appendChild(save)
      row.appendChild(options)
      row.appendChild(routesPanel(project))
      root.appendChild(row)
    })
  }

  function load() {
    fetch("/api/ai/projects", { credentials: "same-origin" })
      .then(function (response) {
        if (!response.ok) throw new Error("load failed")
        return response.json()
      })
      .then(function (data) {
        render(Array.isArray(data.projects) ? data.projects : [])
      })
      .catch(function () {
        root.textContent = ""
        var error = document.createElement("p")
        error.className = "py-3 text-xs text-red-700 dark:text-red-300"
        error.textContent = t("js.ai.projects.load_failed")
        root.appendChild(error)
      })
  }

  if (seed) {
    seed.addEventListener("click", function () {
      seed.disabled = true
      fetch("/api/ai/projects/defaults", { method: "POST", credentials: "same-origin" })
        .then(function (response) {
          if (!response.ok) throw new Error("seed failed")
          return response.json()
        })
        .then(function (data) {
          render(Array.isArray(data.projects) ? data.projects : [])
          setStatus(t("js.ai.projects.defaults_ready"), false)
        })
        .catch(function () {
          setStatus(t("js.ai.projects.defaults_failed"), true)
        })
        .then(function () {
          seed.disabled = false
        })
    })
  }

  load()
})()
