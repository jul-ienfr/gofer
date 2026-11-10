(function () {
  "use strict"

  var form = document.querySelector("[data-ai-token-form]")
  var list = document.querySelector("[data-ai-tokens]")
  var status = document.querySelector("[data-ai-tokens-status]")
  var result = document.querySelector("[data-ai-token-result]")
  var secret = document.querySelector("[data-ai-token-secret]")
  if (!form || !list) return

  function setStatus(message, isError) {
    if (!status) return
    status.textContent = message
    status.classList.toggle("text-red-700", !!isError)
    status.classList.toggle("dark:text-red-300", !!isError)
  }

  function t(key, params) {
    return window.goferT ? window.goferT(key, params) : key
  }

  function load() {
    fetch("/api/ai/tokens", { credentials: "same-origin" })
      .then(function (response) {
        if (!response.ok) throw new Error("load failed")
        return response.json()
      })
      .then(function (data) {
        list.textContent = ""
        var tokens = Array.isArray(data.tokens) ? data.tokens : []
        if (!tokens.length) {
          var empty = document.createElement("p")
          empty.className = "py-3 text-xs text-muted-foreground"
          empty.textContent = t("js.ai.tokens.empty")
          list.appendChild(empty)
          return
        }
        tokens.forEach(function (token) {
          var row = document.createElement("div")
          row.className = "flex flex-wrap items-center justify-between gap-2 py-2 text-xs"
          var label = document.createElement("span")
          var revoked = token.RevokedAt && new Date(token.RevokedAt).getFullYear() > 1970
          var expires = token.ExpiresAt && new Date(token.ExpiresAt).getFullYear() > 1970
          var expiry = expires
            ? new Date(token.ExpiresAt)
            : null
          var expiryText = expiry
            ? " · " + t("js.ai.tokens.expires", {
                date: window.goferDates ? window.goferDates.date(expiry) : expiry.toLocaleDateString(),
              })
            : ""
          var state = revoked ? " · " + t("js.ai.tokens.revoked") : expiryText
          label.textContent = token.Name + " · " + token.Scopes + state
          row.appendChild(label)
          if (!revoked) {
            var revoke = document.createElement("button")
            revoke.type = "button"
            revoke.className = "rounded border border-border px-2 py-0.5 hover:bg-muted"
            revoke.textContent = t("js.ai.tokens.revoke")
            revoke.addEventListener("click", function () {
              revoke.disabled = true
              fetch("/api/ai/tokens/" + encodeURIComponent(token.ID), {
                method: "DELETE",
                credentials: "same-origin",
              })
                .then(function (response) {
                  if (!response.ok) throw new Error("revoke failed")
                  setStatus(t("js.ai.tokens.revoked_status"), false)
                  load()
                })
                .catch(function () {
                  setStatus(t("js.ai.tokens.revoke_failed"), true)
                  revoke.disabled = false
                })
            })
            row.appendChild(revoke)
          }
          list.appendChild(row)
        })
      })
      .catch(function () {
        list.textContent = t("js.ai.tokens.load_failed")
      })
  }

  form.addEventListener("submit", function (event) {
    event.preventDefault()
    var data = new FormData(form)
    var scopes = []
    ;["read", "draft", "write"].forEach(function (scope) {
      if (data.get("scope_" + scope)) scopes.push(scope)
    })
    var button = form.querySelector("button[type=submit]")
    button.disabled = true
    fetch("/api/ai/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({
        name: String(data.get("name") || ""),
        scopes: scopes,
        expires_in_days: Number(data.get("expires_in_days") || 90),
      }),
    })
      .then(function (response) {
        if (!response.ok) {
          return response.json().then(
            function (payload) { throw new Error((payload && payload.error) || t("js.ai.tokens.create_failed")) },
            function () { throw new Error(t("js.ai.tokens.create_failed")) }
          )
        }
        return response.json()
      })
      .then(function (payload) {
        if (result && secret) {
          secret.textContent = payload.token || ""
          result.classList.remove("hidden")
        }
        form.reset()
        setStatus(t("js.ai.tokens.created"), false)
        load()
      })
      .catch(function (error) {
        setStatus(error.message || t("js.ai.tokens.create_failed"), true)
      })
      .then(function () {
        button.disabled = false
      })
  })

  load()
})()
