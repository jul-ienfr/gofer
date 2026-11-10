(function () {
  "use strict"

  function t(key) {
    return window.goferT ? window.goferT(key) : key
  }

  function statusFor(button) {
    var panel = button.closest("[data-ai-analysis-panel]")
    return panel ? panel.querySelector("[data-ai-draft-status]") : null
  }

  function setStatus(button, message, isError) {
    var status = statusFor(button)
    if (!status) return
    status.textContent = message
    status.classList.toggle("text-red-700", !!isError)
    status.classList.toggle("dark:text-red-300", !!isError)
  }

  document.addEventListener("click", function (event) {
    var undoButton = event.target && event.target.closest
      ? event.target.closest("[data-ai-undo]")
      : null
    if (undoButton && !undoButton.disabled) {
      event.preventDefault()
      var actionID = undoButton.getAttribute("data-ai-undo")
      var messageID = undoButton.getAttribute("data-ai-undo-message")
      var section = undoButton.closest("[data-ai-analysis-panel]")
      var undoStatus = section ? section.querySelector("[data-ai-undo-status]") : null
      undoButton.disabled = true
      if (undoStatus) undoStatus.textContent = t("js.ai.draft.restoring")
      fetch("/api/ai/automation/" + encodeURIComponent(actionID || "") + "/undo", {
        method: "POST",
        credentials: "same-origin",
      })
        .then(function (response) {
          if (!response.ok) throw new Error(t("js.ai.draft.undo_failed"))
          return fetch("/api/ai/messages/" + encodeURIComponent(messageID || "") + "/analysis/panel", {
            credentials: "same-origin",
          })
        })
        .then(function (response) {
          if (!response.ok) throw new Error(t("js.ai.draft.refresh_failed"))
          return response.text()
        })
        .then(function (html) {
          if (section) section.outerHTML = html
        })
        .catch(function (error) {
          if (undoStatus) undoStatus.textContent = error.message || t("js.ai.draft.undo_failed")
          undoButton.disabled = false
        })
      return
    }
    var button = event.target && event.target.closest
      ? event.target.closest("[data-ai-draft]")
      : null
    if (!button || button.disabled) return
    event.preventDefault()
    var messageID = button.getAttribute("data-ai-draft")
    if (!/^\d+$/.test(messageID || "")) return
    button.disabled = true
    setStatus(button, t("js.ai.draft.generating"), false)
    fetch("/api/ai/messages/" + encodeURIComponent(messageID) + "/draft", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({}),
    })
      .then(function (response) {
        if (!response.ok) {
          return response.json().then(
            function (data) {
              throw new Error((data && data.error) || t("js.ai.draft.failed"))
            },
            function () {
              throw new Error(t("js.ai.draft.failed"))
            }
          )
        }
        return response.json()
      })
      .then(function (payload) {
        if (typeof window.openComposePrefill !== "function" || !payload.compose) {
          throw new Error(t("js.ai.draft.composer_unavailable"))
        }
        var values = payload.compose
        values._composeDirty = "true"
        return window.openComposePrefill(values, "reply")
      })
      .then(function (opened) {
        setStatus(button, opened ? t("js.ai.draft.review") : t("js.ai.draft.close_current"), !opened)
      })
      .catch(function (error) {
        setStatus(button, error.message || t("js.ai.draft.failed"), true)
      })
      .then(function () {
        button.disabled = false
      })
  })
})()
